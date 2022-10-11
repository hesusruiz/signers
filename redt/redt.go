package redt

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	ethertypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/hesusruiz/signers/client"
	qtypes "github.com/hesusruiz/signers/types"
	"github.com/pterm/pterm"

	ibftengine "github.com/ethereum/go-ethereum/consensus/istanbul/ibft/engine"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rlp"
	lrucache "github.com/hashicorp/golang-lru"
	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/sha3"
)

type ValInfo struct {
	Operator   string `json:"operator"`
	Enode      string `json:"enode"`
	Address    common.Address
	Signatures int
	Proposals  int
}

var enodes = []*ValInfo{
	{
		Operator: "AST",
		Enode:    "enode://367354e3bb59d015fce31967f5dda5c17cb3b9acc5b571695f94a13f89d2a2c64c3bca28da05b6751a7384c38152752de35787d97e9b8d6062b3371b7a9305c4@188.244.90.2:21000?discport=0",
	},
	{
		Operator: "Alisys",
		Enode:    "enode://3905f943ba5446eba164c07ab5f53a84ce17d74ec4d7591f6ec54b9d7608f57cae7cfdf946616385f59cfb5b910161a1f8520cb6f992bcc0d1ab932601205e91@154.62.228.6:21000?discport=0",
	},
	{
		Operator: "Blocknitive",
		Enode:    "enode://6ee5504399ba5a6cbca15d7dd19c652017af0223f12af875044d103307cca82a8105cfb72455836bd52ba11cdbf5a752007af6000d59d146dade7f3738a4d148@185.170.96.121:21000?discport=0",
	},
	{
		Operator: "COUNCILBOX",
		Enode:    "enode://a7e28844702e519f504802a0b45638049db8bf08e18d12e0713c9e5c5707bfabb029583a87e94f8985f9584bee9257a7efe5e057ea61e6b5a16f1eb0b9b3623a@52.232.74.132:21000?discport=0",
	},
	{
		Operator: "DigitelTS",
		Enode:    "enode://32ed5766ac0c482bd7f950087c389710e31b75bf6a06628820f224ddd2fce216b26b113836a456832bbb62a9521d768923dcb8c1c6cbddd06071fa27e1688a1c@176.34.235.103:21000?discport=0",
	},
	{
		Operator: "IN2",
		Enode:    "enode://0ede782b7ce6c7398f100ef33aef6c266972dac19910b5aac1c1eededccd7b4769e7df69e4314927417bbdd9592fc9f583c36274976af29e432b8e64059adc03@15.236.56.133:21000?discport=0",
	},
	{
		Operator: "Izertis",
		Enode:    "enode://51bff825ab4169bc94035fb733a2613018e012460d683a032a20a2a8d305b5eb9462ad7f84ea0e7ce8eec1e0ba0647d5212912016917033c20939719397247a5@3.248.201.125:21000?discport=0",
	},
	{
		Operator: "SERES",
		Enode:    "enode://b9992efd63318b9f41028ec3390abb21bb8fe8f99b0acc30bfccf6d033828b65f65971c1e4975bf41ea9910c879ec9d5df52f24b6bec9058c2cbdb70774b732a@141.144.251.87:21000?discport=0",
	},
	{
		Operator: "Indra",
		Enode:    "enode://7adf7393d3d75978b3d9bf2f78436bb070e1c19eff20eb2eef07dc8293293c4ecbbbcca5a2f84ee6ca9331e8efe7d7d5662ed1f92bb96a6bd0e850715b45ed6d@20.107.215.166:21000?discport=0",
	},
}

// sigHash returns the hash which is used as input for the Istanbul
// signing. It is the hash of the entire header apart from the 65 byte signature
// contained at the end of the extra data.
func sigHash(header *ethertypes.Header) (hash common.Hash) {
	hasher := sha3.NewLegacyKeccak256()
	rlp.Encode(hasher, ethertypes.IstanbulFilteredHeader(header, false))
	hasher.Sum(hash[:0])
	return hash
}

func toBlockNumArg(number int64) string {
	if number == -1 {
		return "latest"
	}
	return fmt.Sprintf("0x%X", number)
}

type RedTNode struct {
	countersLock       sync.RWMutex
	headerCache        *lrucache.Cache
	cli                *ethclient.Client
	rpccli             *rpc.Client
	ctx                context.Context
	valSet             []common.Address
	allValidators      map[common.Address]*ValInfo
	asProposer         map[common.Address]int
	asSigner           map[common.Address]int
	lastBlockProcessed int64
	spinner            *pterm.SpinnerPrinter
}

func NewRedTNode(url string) (*RedTNode, error) {

	// Connect to Client
	rpccli, err := rpc.Dial(url)
	if err != nil {
		return nil, err
	}
	rt := &RedTNode{}
	rt.rpccli = rpccli
	rt.cli = ethclient.NewClient(rpccli)
	rt.ctx = context.Background()

	// Load the current Validator set. It has to be refreshed in case
	// a new Validator is added or removed (an infrequent event)
	// Restarting the program loads the most recent Validator set
	rt.valSet, err = rt.getValSet()
	if err != nil {
		panic(err)
	}

	// Calculate the full validator list including the ones not currently in the valSet
	// Initialise Validators map
	rt.allValidators = make(map[common.Address]*ValInfo, len(enodes))
	for _, item := range enodes {
		en := enode.MustParse(item.Enode)
		address := crypto.PubkeyToAddress(*en.Pubkey())

		item.Address = address

		rt.allValidators[address] = item
	}

	// Initialise the counters for validators/signers
	rt.asProposer = map[common.Address]int{}
	rt.asSigner = map[common.Address]int{}

	for _, addr := range rt.valSet {
		rt.asProposer[addr] = 0
		rt.asSigner[addr] = 0
	}

	// Initialise the header cache
	rt.headerCache, err = lrucache.New(100)
	if err != nil {
		panic(err)
	}

	return rt, nil
}

func (rt *RedTNode) EthClient() *ethclient.Client {
	return rt.cli
}

func (rt *RedTNode) RpcClient() *rpc.Client {
	return rt.rpccli
}

func (rt *RedTNode) InitializeStats(numBlocks int64) {

	// Only us
	rt.countersLock.Lock()
	defer rt.countersLock.Unlock()

	// Reset counters for all Validators
	for _, addr := range rt.valSet {
		rt.asProposer[addr] = 0
		rt.asSigner[addr] = 0
	}

	// Short-circuit if no work
	if numBlocks <= 0 {
		return
	}

	// Get the current block header
	header, err := rt.HeaderByNumber(-1)
	if err != nil {
		log.Fatal().Err(err).Msg("")
	}
	currentNumber := header.Number.Int64()

	// Calculate the ancient block where calculation starts
	oldNumber := currentNumber - numBlocks

	for i := oldNumber; i <= currentNumber; i++ {

		// Get block header data
		header, err = rt.HeaderByNumber(i)
		if err != nil {
			log.Fatal().Err(err).Msg("")
		}

		rt.UpdateStatisticsForBlock(header)

	}

	// Store the current number so several threads in parallel do not alter the statistics
	rt.lastBlockProcessed = currentNumber

}

func (rt *RedTNode) UpdateStatisticsForBlock(header *ethertypes.Header) (author common.Address, signers []common.Address, err error) {

	author, signers, err = SignersFromBlock(header)
	if err != nil {
		log.Fatal().Err(err).Msg("")
	}

	// Only us
	rt.countersLock.Lock()
	defer rt.countersLock.Unlock()

	// Check if the block was already processed
	thisBlockNumber := header.Number.Int64()
	if thisBlockNumber <= rt.lastBlockProcessed {
		return author, signers, nil
	}

	// Increment the counter for authors
	rt.asProposer[author] += 1

	// Increment counters for signers
	for _, seal := range signers {
		// Increment the counter of signatures
		rt.asSigner[seal] += 1
	}

	return author, signers, err

}

func (rt *RedTNode) HeaderByNumber(number int64) (*ethertypes.Header, error) {
	var head *ethertypes.Header

	// Try to get the header from the cache
	cachedHeader, _ := rt.headerCache.Get(number)

	if cachedHeader != nil {
		return cachedHeader.(*ethertypes.Header), nil
	}

	// We are going to call the Geth API, with a timeout of 30 seconds
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := rt.rpccli.CallContext(ctx, &head, "eth_getBlockByNumber", toBlockNumArg(number), false)
	if err == nil && head == nil {
		err = ethereum.NotFound
		return nil, err
	}

	// Add it to the cache
	rt.headerCache.Add(head.Number.Int64(), head)

	return head, err
}

func (rt *RedTNode) CurrentBlockNumber() (int64, error) {
	header, err := rt.HeaderByNumber(-1)
	if err != nil {
		return 0, err
	}
	number := header.Number.Int64()
	return number, nil
}

func (rt *RedTNode) NodeInfo() (*p2p.NodeInfo, error) {
	var ni *p2p.NodeInfo

	// We are going to call the Geth API, with a timeout of 30 seconds
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := rt.rpccli.CallContext(ctx, &ni, "admin_nodeInfo")
	if err == nil && ni == nil {
		err = ethereum.NotFound
		return nil, err
	}
	return ni, err

}

func (rt *RedTNode) Peers() ([]*p2p.PeerInfo, error) {
	var peers []*p2p.PeerInfo

	// We are going to call the Geth API, with a timeout of 30 seconds
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := rt.rpccli.CallContext(ctx, &peers, "admin_peers")
	if err == nil && peers == nil {
		err = ethereum.NotFound
		return nil, err
	}
	return peers, err
}

func (rt *RedTNode) Validators() []common.Address {
	return rt.valSet
}

func (rt *RedTNode) ValidatorInfo(validator common.Address) *ValInfo {
	return rt.allValidators[validator]
}

func (rt *RedTNode) DisplayMyInfo() {

	ni, err := rt.NodeInfo()
	if err != nil {
		log.Fatal().Err(err).Msg("")
	}

	fmt.Printf("About my node:\n")
	out, err := json.MarshalIndent(ni, "", "  ")
	if err != nil {
		log.Fatal().Err(err).Msg("")
	}
	fmt.Printf("%v\n\n", string(out))

}

func (rt *RedTNode) DisplayPeersInfo() {

	peers, err := rt.Peers()
	if err != nil {
		log.Fatal().Err(err).Msg("")
	}
	fmt.Printf("About my peers:\n")
	out, err := json.MarshalIndent(peers, "", "  ")
	if err != nil {
		log.Fatal().Err(err).Msg("")
	}
	fmt.Printf("%v\n\n", string(out))

}

func (rt *RedTNode) getValSet() ([]common.Address, error) {

	var vals []string

	// We are going to call the Geth API, with a timeout of 30 seconds
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := rt.rpccli.CallContext(ctx, &vals, "istanbul_getValidators", toBlockNumArg(-1))
	if err != nil {
		return nil, err
	}

	// In order to have the same order as in the IBFT consensus algorithm,
	// we have to sort addresses in string format by lexicographic order.
	// But the hex strings to order should be in the checked address Ethereum format
	// where some hex letters are uppercase and some are lowercase.
	// This is important because the letter "E" goes before letter "a", for example

	// First we convert the strings into Ethereum Addresses
	valSet := make([]common.Address, len(vals))
	for i, addrStr := range vals {
		valSet[i] = common.HexToAddress(addrStr)
	}

	// Now convert them back into strings but in Ethereum format
	for i := range vals {
		vals[i] = valSet[i].String()
	}

	// Sort the resulting slice lexicographically
	sort.Strings(vals)

	// And finally get the slice with Addresses in the right order
	for i, addrStr := range vals {
		valSet[i] = common.HexToAddress(addrStr)
	}

	return valSet, nil

}

func SignersFromBlock(header *ethertypes.Header) (author common.Address, signers []common.Address, err error) {

	// Retrieve the signature from the header extra-data
	extra, err := ethertypes.ExtractIstanbulExtra(header)
	if err != nil {
		log.Fatal().Err(err).Msg("")
	}

	author, err = istanbul.GetSignatureAddress(sigHash(header).Bytes(), extra.Seal)
	if err != nil {
		log.Fatal().Err(err).Msg("")
	}

	committedSeal := extra.CommittedSeal
	proposalSeal := ibftengine.PrepareCommittedSeal(header.Hash())

	// Get committed seals from current header
	for _, seal := range committedSeal {
		// Get the original address by seal and parent block hash
		addr, err := istanbul.GetSignatureAddress(proposalSeal, seal)
		if err != nil {
			log.Fatal().Err(err).Msg("")
		}
		signers = append(signers, addr)
	}

	return
}

func (rt *RedTNode) DisplaySignersForBlockNumber(number int64, latestTimestamp uint64) uint64 {

	if rt.spinner != nil && rt.spinner.IsActive {
		rt.spinner.Stop()
	}

	// Get the block timestamp with the specified number
	currentHeader, err := rt.HeaderByNumber(number)
	if err != nil {
		log.Fatal().Err(err).Msg("")
	}
	currentTimestamp := currentHeader.Time

	// Calculate the elapsed time with respect to the latest one we received
	elapsed := currentTimestamp - latestTimestamp

	// Update the statistics in memory
	author, signers, err := rt.UpdateStatisticsForBlock(currentHeader)
	if err != nil {
		log.Fatal().Err(err).Msg("")
	}

	// Get the name of the node operator
	oper := rt.ValidatorInfo(author)

	// Determine the next node that should be proposer, according to the round-robin
	// selection algorithm
	var nextIndex int
	for i := 0; i < len(rt.valSet); i++ {
		if author == rt.valSet[i] {
			nextIndex = (i + 1) % len(rt.valSet)
			break
		}
	}
	nextProposer := rt.valSet[nextIndex]

	// Get the name of the node operator
	nextProposerOperator := rt.ValidatorInfo(nextProposer).Operator

	t := time.Unix(int64(currentTimestamp), 0)

	blockInfo := pterm.DefaultBox.WithTitle("Block")

	// Build the header message, in red if block time was bad
	headerMsg1 := pterm.Sprintf("%v (%v sec) %v\n", number, elapsed, t)
	if elapsed > 5 {
		headerMsg1 = pterm.Red(headerMsg1)
	}

	// The author info
	headerMsg2 := pterm.Sprintf("Author: %v (%v) (%v)\n", oper.Operator, rt.asProposer[author], author)

	// Gas limit and number of txs
	headerMsg3 := pterm.Sprintf("GasLimit: %v GasUsed: %v\n", currentHeader.GasLimit, currentHeader.GasUsed)

	var currentSigners = map[common.Address]bool{}

	for _, seal := range signers {
		currentSigners[seal] = true
	}

	tableMsg := ""

	// Print the title of the table
	tableMsg += pterm.Sprintf("\n  Author |  Signer  |       Name      Address")

	for _, val := range rt.Validators() {

		item := rt.ValidatorInfo(val)

		authorCount := rt.asProposer[item.Address]

		var authorCountStr string
		if authorCount == 0 {
			authorCountStr = pterm.FgRed.Sprintf("%6v", authorCount)
		} else {
			authorCountStr = pterm.Sprintf("%6v", authorCount)
		}

		var currentAuthorStr string
		if author.Hex() == item.Address.Hex() {
			currentAuthorStr = pterm.BgLightBlue.Sprint(pterm.Bold.Sprintf("%v %1v", authorCountStr, "X"))
		} else {
			currentAuthorStr = pterm.Bold.Sprintf("%v %1v", authorCountStr, " ")
		}

		signerCount := rt.asSigner[item.Address]

		var signerCountStr string
		if signerCount == 0 {
			signerCountStr = pterm.FgRed.Sprintf("%6v", signerCount)
		} else {
			signerCountStr = pterm.Sprintf("%6v", signerCount)
		}

		var currentSignerStr string
		if currentSigners[item.Address] {
			currentSignerStr = pterm.BgLightBlue.Sprint(pterm.Bold.Sprintf("%v %1v", signerCountStr, "X"))
		} else {
			currentSignerStr = pterm.Bold.Sprintf("%v %1v", signerCountStr, " ")
		}

		tableMsg += pterm.Sprintf("\n%v | %v | %12v %v", currentAuthorStr, currentSignerStr, item.Operator, item.Address)

	}

	blockInfo.Println(headerMsg1 + headerMsg2 + headerMsg3 + tableMsg)

	rt.spinner, _ = pterm.DefaultSpinner.Start("Waiting for ", nextProposerOperator, " to create next block ...")
	rt.spinner.RemoveWhenDone = true

	return currentTimestamp

}

type SignerData struct {
	Proposer string
	Signers  []string
}

func (rt *RedTNode) SignerDataForBlockNumber(number int64) (*ethertypes.Header, *SignerData, error) {

	data := &SignerData{}

	// Get the block timestamp with the specified number
	header, err := rt.HeaderByNumber(number)
	if err != nil {
		return nil, nil, err
	}

	author, signers, err := SignersFromBlock(header)
	if err != nil {
		return nil, nil, err
	}

	data.Signers = make([]string, len(signers))

	data.Proposer = author.String()
	for i, item := range signers {
		data.Signers[i] = item.String()
	}

	return header, data, nil

}

func (rt *RedTNode) SignersForHeader(header *ethertypes.Header, latestTimestamp uint64) (map[string]any, uint64) {

	data := make(map[string]any)

	currentTimestamp := header.Time

	// Calculate the elapsed time with respect to the latest one we received
	elapsed := currentTimestamp - latestTimestamp

	// Update the statistics in memory
	author, signers, err := rt.UpdateStatisticsForBlock(header)
	if err != nil {
		log.Fatal().Err(err).Msg("")
	}

	// Get the name of the node operator
	oper := rt.ValidatorInfo(author)

	// Determine the next node that should be proposer, according to the round-robin
	// selection algorithm
	var nextIndex int
	for i := 0; i < len(rt.valSet); i++ {
		if author == rt.valSet[i] {
			nextIndex = (i + 1) % len(rt.valSet)
			break
		}
	}
	nextProposer := rt.valSet[nextIndex]

	// Get the name of the node operator
	nextProposerOperator := rt.ValidatorInfo(nextProposer).Operator

	t := time.Unix(int64(currentTimestamp), 0)

	// Lock for reading
	rt.countersLock.RLock()
	defer rt.countersLock.RUnlock()

	data["number"] = header.Number
	data["elapsed"] = elapsed
	data["timestamp"] = t
	data["operator"] = oper.Operator
	data["authorCount"] = rt.asProposer[author]
	data["authorAddress"] = author
	data["nextProposerOperator"] = nextProposerOperator

	data["gasLimit"] = header.GasLimit
	data["gasUsed"] = header.GasUsed

	var currentSigners = map[common.Address]bool{}

	for _, seal := range signers {
		currentSigners[seal] = true
	}

	st := make([]map[string]any, len(rt.Validators()))

	for i, val := range rt.Validators() {

		d := make(map[string]any)

		item := rt.ValidatorInfo(val)

		authorCount := rt.asProposer[item.Address]

		authorCountStr := fmt.Sprintf("%v", authorCount)
		if authorCount == 0 {
			authorCountStr = fmt.Sprintf("<span class='w3-badge w3-red'>%v</span>", authorCount)
		}
		if author.Hex() == item.Address.Hex() {
			authorCountStr = fmt.Sprintf("<span class='w3-badge'>%v</span>", authorCount)
		}

		d["authorCount"] = authorCountStr

		signerCount := rt.asSigner[item.Address]

		signerCountStr := fmt.Sprintf("%v", signerCount)
		if signerCount == 0 {
			signerCountStr = fmt.Sprintf("<span class='w3-badge w3-red'>%v</span>", signerCount)
		}
		if currentSigners[item.Address] {
			signerCountStr = fmt.Sprintf("<span class='w3-badge'>%v</span>", signerCount)
		}

		d["signerCount"] = signerCountStr

		d["operator"] = item.Operator
		d["address"] = item.Address

		st[i] = d

	}

	data["signers"] = st

	return data, currentTimestamp

}

/*
	 func (rt *RedTNode) SignersForBlockNumber(number int64, latestTimestamp uint64) (map[string]any, uint64) {

		data := make(map[string]any)

		// Get the block timestamp with the specified number
		currentHeader, err := rt.HeaderByNumber(number)
		if err != nil {
			log.Fatal().Err(err).Msg("")
		}
		currentTimestamp := currentHeader.Time

		// Calculate the elapsed time with respect to the latest one we received
		elapsed := currentTimestamp - latestTimestamp

		// Update the statistics in memory
		author, signers, err := rt.UpdateStatisticsForBlock(currentHeader)
		if err != nil {
			log.Fatal().Err(err).Msg("")
		}

		// Get the name of the node operator
		oper := rt.ValidatorInfo(author)

		// Determine the next node that should be proposer, according to the round-robin
		// selection algorithm
		var nextIndex int
		for i := 0; i < len(rt.valSet); i++ {
			if author == rt.valSet[i] {
				nextIndex = (i + 1) % len(rt.valSet)
				break
			}
		}
		nextProposer := rt.valSet[nextIndex]

		// Get the name of the node operator
		nextProposerOperator := rt.ValidatorInfo(nextProposer).Operator

		t := time.Unix(int64(currentTimestamp), 0)

		data["number"] = currentHeader.Number
		data["elapsed"] = elapsed
		data["timestamp"] = t
		data["operator"] = oper.Operator
		data["authorCount"] = rt.asProposer[author]
		data["authorAddress"] = author
		data["nextProposerOperator"] = nextProposerOperator

		data["gasLimit"] = currentHeader.GasLimit
		data["gasUsed"] = currentHeader.GasUsed

		var currentSigners = map[common.Address]bool{}

		for _, seal := range signers {
			currentSigners[seal] = true
		}

		st := make([]map[string]any, len(rt.Validators()))

		for i, val := range rt.Validators() {

			d := make(map[string]any)

			item := rt.ValidatorInfo(val)

			authorCount := rt.asProposer[item.Address]

			authorCountStr := fmt.Sprintf("%v", authorCount)
			if authorCount == 0 {
				authorCountStr = fmt.Sprintf("<span class='w3-badge w3-red'>%v</span>", authorCount)
			}
			if author.Hex() == item.Address.Hex() {
				authorCountStr = fmt.Sprintf("<span class='w3-badge'>%v</span>", authorCount)
			}

			d["authorCount"] = authorCountStr

			signerCount := rt.asSigner[item.Address]

			signerCountStr := fmt.Sprintf("%v", signerCount)
			if signerCount == 0 {
				signerCountStr = fmt.Sprintf("<span class='w3-badge w3-red'>%v</span>", signerCount)
			}
			if currentSigners[item.Address] {
				signerCountStr = fmt.Sprintf("<span class='w3-badge'>%v</span>", signerCount)
			}

			d["signerCount"] = signerCountStr

			d["operator"] = item.Operator
			d["address"] = item.Address

			st[i] = d

		}

		data["signers"] = st

		return data, currentTimestamp

}
*/
func MonitorSigners(url string, numBlocks int64, refresh int64) {

	// Connect to the RedT node
	rt, err := NewRedTNode(url)
	if err != nil {
		log.Fatal().Err(err).Msg("")
		os.Exit(1)
	}

	rt.spinner, _ = pterm.DefaultSpinner.Start("Calculating statistics for ", numBlocks, " blocks ...")
	rt.spinner.RemoveWhenDone = true

	// Initialise statistics with historic info
	rt.InitializeStats(numBlocks)

	rt.spinner.Stop()

	// Get the current block header info
	latestHeader, err := rt.HeaderByNumber(-1)
	if err != nil {
		log.Fatal().Err(err).Msg("")
	}
	latestNumber := latestHeader.Number.Int64()

	oldNumber := latestNumber - 0

	// Start from an old block
	latestHeader, err = rt.HeaderByNumber(oldNumber)
	if err != nil {
		log.Fatal().Err(err).Msg("")
	}
	latestNumber = latestHeader.Number.Int64()

	latestTimestamp := latestHeader.Time

	// Display  for the first time
	rt.DisplaySignersForBlockNumber(latestNumber, latestTimestamp)

	for {

		// Sleep before getting the next one
		time.Sleep(time.Duration(refresh) * time.Second)

		// Get the current block number
		currentHeader, err := rt.HeaderByNumber(-1)
		if err != nil {
			log.Fatal().Err(err).Msg("")
		}
		currentNumber := currentHeader.Number.Int64()

		// Make sure we have advanced at least one block
		if currentNumber == latestNumber {
			continue
		}

		// Display all blocks from the latest one until the current one
		for i := latestNumber + 1; i <= currentNumber; i++ {
			latestTimestamp = rt.DisplaySignersForBlockNumber(i, latestTimestamp)
		}

		// Update the latest block number that we processed
		latestNumber = currentNumber

	}

}

func MonitorSignersWS(url string, numBlocks int64) {

	// Connect to the RedT node
	rt, err := NewRedTNode(url)
	if err != nil {
		log.Fatal().Err(err).Msg("")
		os.Exit(1)
	}

	qc, err := client.NewQuorumClient(url)
	if err != nil {
		log.Fatal().Err(err).Msg("")
	}
	defer qc.Stop()

	inputCh := make(chan qtypes.RawHeader)

	err = qc.SubscribeChainHead(inputCh)
	if err != nil {
		log.Fatal().Err(err).Msg("")
	}

	latestTimestamp := uint64(0)

	isFirst := true

	for {
		select {
		case header := <-inputCh:
			if isFirst {
				// Do not display, we just get its timestamp
				// Get the block timestamp
				currentHeader, err := rt.HeaderByNumber(int64(header.Number))
				if err != nil {
					// Log the error and retry with next block
					log.Error().Err(err).Msg("")
					continue
				}
				latestTimestamp = currentHeader.Time
				isFirst = false
				continue
			}
			latestTimestamp = rt.DisplaySignersForBlockNumber(int64(header.Number), latestTimestamp)
		}
	}

}

func DisplayPeersInfo(url string) {

	// Connect to the RedT node
	rt, err := NewRedTNode(url)
	if err != nil {
		log.Fatal().Err(err).Msg("")
		os.Exit(1)
	}

	rt.DisplayPeersInfo()

}
