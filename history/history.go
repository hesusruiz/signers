package history

import (
	"fmt"
	"math/big"
	"sync"
	"time"

	"database/sql"

	_ "github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/hesusruiz/signers/redt"
	"github.com/labstack/gommon/log"
	"github.com/syndtr/goleveldb/leveldb/errors"

	_ "github.com/mattn/go-sqlite3"
)

// **************************************
// The Blockchain table
// **************************************

var blockchainTableCreateStmt = `
CREATE TABLE IF NOT EXISTS blockchain (
  Number        INTEGER PRIMARY KEY,
  Proposer      TEXT,
  ProposerCount INTEGER,
  GasLimit      INTEGER,
  GasUsed       INTEGER,
  Time          INTEGER,
  NumTxs        INTEGER,
  ParentHash    TEXT,
  TxHash        TEXT,
  ReceiptHash   TEXT
);`

// Dropping the table
var blockchainTableDropStmt = `DROP TABLE IF EXISTS blockchain`

// Insert a record into the table
var blockchainTableInsertRecordStmt = `INSERT INTO blockchain VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

// **************************************
// The Signers table
// **************************************

var signersTableCreateStmt = `
CREATE TABLE IF NOT EXISTS signers (
  Number      INTEGER,
  Address     TEXT,
  AsProposer  INTEGER,
  AsSigner    INTEGER
);`

// Dropping the table
var signersTableDropStmt = `DROP TABLE IF EXISTS signers`

// Insert a record into the table
var signersTableInsertRecordStmt = `INSERT INTO signers VALUES (?, ?, ?, ?)`

type Blockchain struct {
	db                            *sql.DB
	tx                            *sql.Tx
	rt                            *redt.RedTNode
	mu                            sync.Mutex
	blockchainTableInsertPrepared *sql.Stmt
	signersTableInsertPrepared    *sql.Stmt
}

const defaultDBName = "./blockchain.sqlite?_journal=WAL"

var defaultWallet *Blockchain

// Open creates or opens the database file and creates the tables if not yet done
func Open(name string) (b *Blockchain, err error) {

	// Use default file name if not provided
	if len(name) == 0 {
		name = defaultDBName
	}

	// Open the database (create it if it does not exists)
	db, err := sql.Open("sqlite3", name)
	if err != nil {
		log.Error(err)
		return nil, err
	}

	// Create the main table
	err = openOrCreateTable(db, blockchainTableCreateStmt)
	if err != nil {
		log.Error(err)
		return nil, err
	}

	// Create the signers table
	err = openOrCreateTable(db, signersTableCreateStmt)
	if err != nil {
		log.Error(err)
		return nil, err
	}

	b = &Blockchain{
		db: db,
	}

	return b, err
}

func openOrCreateTable(db *sql.DB, st string) error {
	stmt, err := db.Prepare(st)
	if err != nil {
		log.Error(err)
		return err
	}
	_, err = stmt.Exec()
	if err != nil {
		log.Error(err)
		return err
	}
	return nil
}

func (b *Blockchain) MinBlockNumber() (int64, error) {

	var number int64

	err := b.db.QueryRow("SELECT MIN(number) FROM blockchain").Scan(&number)
	if err != nil {
		log.Error(err)
		return 0, err
	}

	return number, nil
}

func (b *Blockchain) MaxBlockNumber() (int64, error) {

	var number int64

	err := b.db.QueryRow("SELECT MAX(number) FROM blockchain").Scan(&number)
	if err != nil {
		log.Error(err)
		return 0, err
	}

	return number, nil
}

func (b *Blockchain) TimestampForNumber(number int64) (int64, error) {

	var timestamp int64

	err := b.db.QueryRow("SELECT time FROM blockchain WHERE number=?", number).Scan(&timestamp)
	if err != nil {
		log.Error(err)
		return 0, err
	}

	return timestamp, nil
}

// Begin starts a new transaction
func (b *Blockchain) Begin() error {

	// Start the transaction
	tx, err := b.db.Begin()
	if err != nil {
		log.Error(err)
		return err
	}
	b.tx = tx

	// Prepare the statements to improve performance
	stmt, err := b.tx.Prepare(blockchainTableInsertRecordStmt)
	if err != nil {
		log.Error(err)
		return err
	}
	b.blockchainTableInsertPrepared = stmt

	stmt, err = b.tx.Prepare(signersTableInsertRecordStmt)
	if err != nil {
		log.Error(err)
		return err
	}
	b.signersTableInsertPrepared = stmt

	return nil
}

func (b *Blockchain) Commit() error {
	err := b.tx.Commit()
	if err != nil {
		log.Error(err)
		return err
	}
	b.blockchainTableInsertPrepared.Close()
	b.signersTableInsertPrepared.Close()
	return nil
}

func (b *Blockchain) InsertHeader(h *types.Header, signers *redt.SignerData, numtxs uint64) error {

	// Number        INTEGER PRIMARY KEY,
	// Proposer      TEXT,
	// ProposerCount INTEGER,
	// GasLimit      INTEGER,
	// GasUsed       INTEGER,
	// Time          INTEGER,
	// NumTxs        INTEGER,
	// ParentHash    TEXT,
	// TxHash        TEXT,
	// ReceiptHash   TEXT,

	_, err := b.blockchainTableInsertPrepared.Exec(
		h.Number.Uint64(),
		signers.Proposer,
		0,
		h.GasLimit,
		h.GasUsed,
		h.Time,
		numtxs,
		h.ParentHash.Bytes(),
		h.TxHash.Bytes(),
		h.ReceiptHash.Bytes(),
	)
	if err != nil {
		log.Error(err)
		return err
	}

	for _, address := range signers.Signers {
		// Number      INTEGER PRIMARY KEY,
		// Address     TEXT,
		// AsProposer  INTEGER,
		// AsSigner    INTEGER,

		_, err = b.signersTableInsertPrepared.Exec(
			h.Number.Uint64(),
			address,
			0,
			0,
		)
		if err != nil {
			log.Error(err)
			return err
		}

	}

	return nil
}

// GetBlockForNumber gets a block with specified number ither from the database or from the network
// It updates de database if the block is not there
func (b *Blockchain) SignerDataForBlockNumberCached(number int64) (*types.Header, *redt.SignerData, error) {

	// The call is serialised across goroutines
	b.mu.Lock()
	defer b.mu.Unlock()

	// Check that both connection to RedT and the local database are initialised
	if b.rt == nil || b.db == nil {
		err := errors.New("connection to RedT or database not initialised")
		log.Error(err)
		return nil, nil, err
	}

	// Number        INTEGER PRIMARY KEY,
	// Proposer      TEXT,
	// ProposerCount INTEGER,
	// GasLimit      INTEGER,
	// GasUsed       INTEGER,
	// Time          INTEGER,

	// Check if block is in the database
	var proposer string
	var proposercount int64
	var gaslimit, gasused, timestamp uint64

	err := b.db.QueryRow("SELECT proposer, proposercount, gaslimit, gasused, time FROM blockchain WHERE number=?", number).Scan(&proposer, &proposercount, &gaslimit, &gasused, &timestamp)
	if err != nil && err != sql.ErrNoRows {
		log.Error(err)
		return nil, nil, err
	}

	// If not found in database, retrieve it from the network
	if err == sql.ErrNoRows {

		// Get the block data from network
		header, signers, err := b.rt.SignerDataForBlockNumber(number)
		if err != nil {
			log.Error(err)
			return nil, nil, err
		}

		// Start a db transaction
		err = b.Begin()
		if err != nil {
			log.Error(err)
			return nil, nil, err
		}
		defer b.db.Close()

		// Insert the record in the db
		err = b.InsertHeader(header, signers, 0)
		if err != nil {
			log.Error(err)
			return nil, nil, err
		}

		// Commit the transaction
		err = b.Commit()
		if err != nil {
			log.Error(err)
			return nil, nil, err
		}

		// Return the data to the caller
		return header, signers, nil

	}

	// We have to build the signer data from the database
	header := &types.Header{}

	num := new(big.Int).SetInt64(number)
	header.Number = num
	header.Time = timestamp
	header.GasLimit = gaslimit
	header.GasUsed = gasused

	signers := &redt.SignerData{}
	signers.Proposer = proposer

	// CREATE TABLE IF NOT EXISTS signers (
	// 	Number      INTEGER,
	// 	Address     TEXT,
	// 	AsProposer  INTEGER,
	// 	AsSigner    INTEGER
	// );`

	// Retrieve the signers data from the signers table
	rows, err := b.db.Query("SELECT address FROM signers WHERE number=?", number)
	if err != nil {
		log.Error(err)
		return nil, nil, err
	}
	defer rows.Close()

	sgs := make([]string, 0)

	// Retrieve all signers
	for rows.Next() {

		var address string

		err = rows.Scan(&address)
		if err != nil {
			log.Error(err)
			return nil, nil, err
		}
		sgs = append(sgs, address)

	}
	signers.Signers = sgs

	// Ensure to check for Close errors that may be returned from the driver. The query may
	// encounter an auto-commit error and be forced to rollback changes.
	if err := rows.Close(); err != nil {
		log.Error(err)
		return nil, nil, err
	}

	// Rows.Err will report the last error encountered by Rows.Scan.
	if err := rows.Err(); err != nil {
		log.Error(err)
		return nil, nil, err
	}

	return header, signers, nil

}

func showStats(dsn string) error {

	// Open the database
	blk, err := Open(dsn)
	if err != nil {
		log.Error(err)
		return err
	}
	defer blk.db.Close()

	var minNumber int64
	var minTimestamp int64
	var maxNumber int64
	var maxTimestamp int64

	minNumber, err = blk.MinBlockNumber()
	if err != nil {
		log.Error(err)
		return err
	}

	minTimestamp, err = blk.TimestampForNumber(minNumber)
	if err != nil {
		log.Error(err)
		return err
	}
	mint := time.Unix(int64(minTimestamp), 0)

	maxNumber, err = blk.MaxBlockNumber()
	if err != nil {
		log.Error(err)
		return err
	}

	maxTimestamp, err = blk.TimestampForNumber(maxNumber)
	if err != nil {
		log.Error(err)
		return err
	}
	maxt := time.Unix(int64(maxTimestamp), 0)

	fmt.Printf("%v blocks from %v (%v) to %v (%v)\n", maxNumber-minNumber, minNumber, mint, maxNumber, maxt)

	return nil

}

func HistoryForward(url string, dsn string, stats bool) error {

	var startNumber int64

	if stats {
		return showStats(dsn)
	}

	// Connect to the RedT node
	rt, err := redt.NewRedTNode(url)
	if err != nil {
		log.Error(err)
		return err
	}

	// Open the database
	blk, err := Open(dsn)
	if err != nil {
		log.Error(err)
		return err
	}
	defer blk.db.Close()

	// Get the maximum block number in the database
	maxNumber, err := blk.MaxBlockNumber()
	if err != nil {
		log.Error(err)
		return err
	}

	// Get the current block number in the blockchain
	currentNumber, err := rt.CurrentBlockNumber()
	if err != nil {
		return err
	}

	// If the database is empty, start from the current blockchain number upwards
	if maxNumber == 0 {

		startNumber = currentNumber

	} else {

		// We will start from the maximum number not yet registered + 1
		startNumber = maxNumber + 1

	}

	// We perform a Commit every 100 insertions, this is the counter
	count := 0
	const insertsPerCommit = 100

	// Start a db transaction
	err = blk.Begin()
	if err != nil {
		return err
	}

	fmt.Printf("Current block: %v Max db block: %v, Start block: %v\n", currentNumber, maxNumber, startNumber)

	// Loop until the current block (at this time)
	for i := startNumber; i <= currentNumber; i++ {

		// Get the block data
		header, signers, err := rt.SignerDataForBlockNumber(i)
		if err != nil {
			return err
		}

		// Insert
		err = blk.InsertHeader(header, signers, 0)
		if err != nil {
			log.Error(err)
			return err
		}

		count++

		// Commit if enough insertions have been made
		if count >= insertsPerCommit {
			fmt.Println("Block ", i)

			// Commit and start a new transaction
			blk.Commit()
			err = blk.Begin()
			if err != nil {
				return err
			}
			count = 0
		}

	}

	// Commit the last set of inserts
	blk.Commit()

	return nil
}

func HistoryBackwards(url string, dsn string, stats bool) error {

	var startNumber int64

	if stats {
		return showStats(dsn)
	}

	// Connect to the RedT node
	rt, err := redt.NewRedTNode(url)
	if err != nil {
		log.Error(err)
		return err
	}

	// Open the database
	blk, err := Open(dsn)
	if err != nil {
		log.Error(err)
		return err
	}
	defer blk.db.Close()

	numRecords, err := blk.MaxBlockNumber()
	if err != nil {
		log.Error(err)
		return err
	}

	// If the database is empty, start from the current blockchain number downwards
	if numRecords == 0 {

		// Get the current block number in the blockchain
		startNumber, err = rt.CurrentBlockNumber()
		if err != nil {
			return err
		}

	} else {

		// We will start from the lowest number not yet registered
		minNumber, err := blk.MinBlockNumber()
		if err != nil {
			log.Error(err)
			return err
		}

		// Check if we have nothing to do
		if minNumber == 0 {
			return errors.New("lowest block number is zero")
		}

		// Set the start number to one lower
		startNumber = minNumber - 1

	}

	// We perform a Commit every 100 insertions, this is the counter
	count := 0
	const insertsPerCommit = 100

	// Start a db transaction
	err = blk.Begin()
	if err != nil {
		return err
	}

	fmt.Println("Start:", startNumber)
	// Loop until the genesis block
	for i := startNumber; i >= 0; i-- {

		// Get the block data
		header, signers, err := rt.SignerDataForBlockNumber(i)
		if err != nil {
			return err
		}

		// Insert
		err = blk.InsertHeader(header, signers, 0)
		if err != nil {
			log.Error(err)
			return err
		}

		count++

		// Commit if enough insertions have been made
		if count >= insertsPerCommit {
			fmt.Println("Block ", i)

			// Commit and start a new transaction
			blk.Commit()
			err = blk.Begin()
			if err != nil {
				return err
			}
			count = 0
		}

	}

	// Commit the last set of inserts
	blk.Commit()

	return nil
}
