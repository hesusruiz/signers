// Copyright 2015 The go-ethereum Authors
// This file is part of go-ethereum.
//
// go-ethereum is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// go-ethereum is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with go-ethereum. If not, see <http://www.gnu.org/licenses/>.

// bootnode runs a bootstrap node for the Ethereum Discovery Protocol.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/pterm/pterm"

	ibftengine "github.com/ethereum/go-ethereum/consensus/istanbul/ibft/engine"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rlp"
	"golang.org/x/crypto/sha3"

	"github.com/rs/zerolog/log"
)

var localNode = "http://127.0.0.1:22000"
var regularNode = "http://15.236.0.91:22000"

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
		Enode:    "enode://51bff825ab4169bc94035fb733a2613018e012460d683a032a20a2a8d305b5eb9462ad7f84ea0e7ce8eec1e0ba0647d5212912016917033c20939719397247a5@54.77.43.225:21000?discport=0",
	},
	{
		Operator: "SERES",
		Enode:    "enode://b9992efd63318b9f41028ec3390abb21bb8fe8f99b0acc30bfccf6d033828b65f65971c1e4975bf41ea9910c879ec9d5df52f24b6bec9058c2cbdb70774b732a@141.144.251.87:21000?discport=0",
	},
	{
		Operator: "Kunfud",
		Enode:    "enode://b38efe0bf1e51d2637b495ab8442992bc116bee011d7730a3d2a8657555b5f039486c4b6b984ca82fcd041e06fa8e39580fd451f55a2c403e97a6a02807e4a3a@109.234.71.8:21000?discport=0",
	},
	{
		Operator: "Indra",
		Enode:    "enode://7adf7393d3d75978b3d9bf2f78436bb070e1c19eff20eb2eef07dc8293293c4ecbbbcca5a2f84ee6ca9331e8efe7d7d5662ed1f92bb96a6bd0e850715b45ed6d@51.104.153.98:21000?discport=0",
	},
}

func clear() {
	fmt.Print("\033[H\033[2J")
}

// sigHash returns the hash which is used as input for the Istanbul
// signing. It is the hash of the entire header apart from the 65 byte signature
// contained at the end of the extra data.
func sigHash(header *types.Header) (hash common.Hash) {
	hasher := sha3.NewLegacyKeccak256()
	rlp.Encode(hasher, types.IstanbulFilteredHeader(header, false))
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
	cli           *ethclient.Client
	rpccli        *rpc.Client
	ctx           context.Context
	valSet        []common.Address
	allValidators map[common.Address]*ValInfo
	asProposer    map[common.Address]int
	asSigner      map[common.Address]int
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
	// a new Validators is added or removed (an infrequent event)
	// Restarting the program loads the most recent Validator set
	rt.valSet, err = rt.getValSet()

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

	return rt, nil
}

func (rt *RedTNode) EthClient() *ethclient.Client {
	return rt.cli
}

func (rt *RedTNode) RpcClient() *rpc.Client {
	return rt.rpccli
}

func (rt *RedTNode) InitializeStats(numBlocks int64) {

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

		rt.updateStatisticsForBlock(header)

	}

}

func (rt *RedTNode) updateStatisticsForBlock(header *types.Header) (author common.Address, signers []common.Address, err error) {

	author, signers, err = signersFromBlock(header)
	if err != nil {
		log.Fatal().Err(err).Msg("")
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

func (rt *RedTNode) HeaderByNumber(number int64) (*types.Header, error) {
	var head *types.Header

	err := rt.rpccli.CallContext(rt.ctx, &head, "eth_getBlockByNumber", toBlockNumArg(number), false)
	if err == nil && head == nil {
		err = ethereum.NotFound
		return nil, err
	}
	return head, err
}

func (rt *RedTNode) NodeInfo() (*p2p.NodeInfo, error) {
	var ni *p2p.NodeInfo

	err := rt.rpccli.CallContext(rt.ctx, &ni, "admin_nodeInfo")
	if err == nil && ni == nil {
		err = ethereum.NotFound
		return nil, err
	}
	return ni, err

}

func (rt *RedTNode) Peers() ([]*p2p.PeerInfo, error) {
	var ni []*p2p.PeerInfo

	err := rt.rpccli.CallContext(rt.ctx, &ni, "admin_peers")
	if err == nil && ni == nil {
		err = ethereum.NotFound
		return nil, err
	}
	return ni, err
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

	err := rt.rpccli.CallContext(rt.ctx, &vals, "istanbul_getValidators", toBlockNumArg(-1))
	if err != nil {
		return nil, err
	}

	//	sort.Strings(vals)

	valSet := make([]common.Address, len(vals))
	for i, addrStr := range vals {
		valSet[i] = common.HexToAddress(addrStr)
	}

	return valSet, nil

}

func signersFromBlock(header *types.Header) (author common.Address, signers []common.Address, err error) {

	// Retrieve the signature from the header extra-data
	extra, err := types.ExtractIstanbulExtra(header)
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
		// 2. Get the original address by seal and parent block hash
		addr, err := istanbul.GetSignatureAddress(proposalSeal, seal)
		if err != nil {
			log.Fatal().Err(err).Msg("")
		}
		signers = append(signers, addr)
	}

	return
}

func (rt *RedTNode) displaySignersForBlockNumber(number int64, latestTimestamp uint64) uint64 {

	// Get the block
	currentHeader, err := rt.HeaderByNumber(number)
	if err != nil {
		log.Fatal().Err(err).Msg("")
	}
	currentTimestamp := currentHeader.Time

	elapsed := currentTimestamp - latestTimestamp

	author, signers, err := rt.updateStatisticsForBlock(currentHeader)
	if err != nil {
		log.Fatal().Err(err).Msg("")
	}

	// Get the operator for the node
	oper := rt.ValidatorInfo(author)

	t := time.Unix(int64(currentTimestamp), 0)

	pterm.Print(pterm.Bold.Sprintf("Block %v (%v sec) %v\n", number, elapsed, t))
	pterm.Printf("Author: %v (%v) (%v)\n", oper.Operator, rt.asProposer[author], author)

	var currentSigners = map[common.Address]bool{}

	for _, seal := range signers {
		currentSigners[seal] = true
	}

	// Print the title of the table
	pterm.Printf("  Author |  Signer  |       Name      Address\n")

	for _, val := range rt.Validators() {

		item := rt.ValidatorInfo(val)

		authorCount := rt.asProposer[item.Address]
		authorCountStr := pterm.Sprintf("%6v", authorCount)
		if authorCount == 0 {
			authorCountStr = pterm.FgRed.Sprintf("%6v", authorCount)
		}

		var currentAuthorStr string
		if author.Hex() == item.Address.Hex() {
			currentAuthorStr = pterm.BgLightBlue.Sprint(pterm.Bold.Sprintf("%v %1v", authorCountStr, "X"))
		} else {
			currentAuthorStr = pterm.Bold.Sprintf("%v %1v", authorCountStr, " ")
		}

		signerCount := rt.asSigner[item.Address]
		signerCountStr := pterm.Sprintf("%6v", signerCount)
		if signerCount == 0 {
			signerCountStr = pterm.FgRed.Sprintf("%6v", signerCount)
		}

		var currentSignerStr string
		if currentSigners[item.Address] {
			currentSignerStr = pterm.BgLightBlue.Sprint(pterm.Bold.Sprintf("%v %1v", signerCountStr, "X"))
		} else {
			currentSignerStr = pterm.Bold.Sprintf("%v %1v", signerCountStr, " ")
		}

		pterm.Printf("%v | %v | %15v %v\n", currentAuthorStr, currentSignerStr, item.Operator, item.Address)

	}
	pterm.Printf("================================================================================\n")

	return currentTimestamp

}

func main() {

	var url = localNode
	args := os.Args

	if len(args) > 1 {
		url = args[1]
	}

	// Connect to the RedT node
	rt, err := NewRedTNode(url)
	if err != nil {
		log.Fatal().Err(err).Msg("")
		os.Exit(1)
	}

	// Initialise statistics with historic info
	rt.InitializeStats(100)

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

	// Display info
	rt.displaySignersForBlockNumber(latestNumber, latestTimestamp)

	for {

		// Sleep before getting the next one
		time.Sleep(2 * time.Second)

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
			latestTimestamp = rt.displaySignersForBlockNumber(i, latestTimestamp)
		}

		// Update the latest block number that we processed
		latestNumber = currentNumber

	}

}
