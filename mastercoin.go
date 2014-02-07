package mscd

import (
	_ "errors"
	"github.com/conformal/btcdb"
	_"log"
	"math"
	_ "github.com/conformal/btcscript"
	"github.com/conformal/btcutil"
	_ "github.com/conformal/btcwire"
	"github.com/mastercoin-MSC/mscutil"
	_"fmt"
	"os"
	"os/user"
	"path"
)

const (
	Debug                   = true
	blockChannelQueueLength = 50

	// The height of the blocks until we reach the block which first included
	// a Tx to the exodus address (i.e. mark of mastercoin's beginning)
	ExodusBlockHeight = 249498
)

var MastercoinBlockChannel chan *btcutil.Block

func QueueMessageForBlockChannel(block *btcutil.Block) {
	MastercoinBlockChannel <- block
}

type MastercoinServer struct {
	// Main quit channel. This is used internally to determine whether
	// certain process need to quit
	quit chan bool

	// Server shutdown channel.
	shutdownChan chan bool

	// Message parser
	msgParser *MsgParser

	btcdb btcdb.Db

	db *mscutil.LDBDatabase

	FundraiserHandler *FundraiserHandler
	SimpleSendHandler *SimpleSendHandler
}

func NewMastercoinServer(btcdb btcdb.Db) *MastercoinServer {
	mscutil.Logger.Println("Creating mastercoin server")
	MastercoinBlockChannel = make(chan *btcutil.Block, blockChannelQueueLength)

	// TODO: Rename this to replay database once we start reimporting
	db, err := mscutil.NewLDBDatabase("db")
	bDb, err := mscutil.NewLDBDatabase("bdb")
	if err != nil {
		panic("Could not create database")
	}

	s := &MastercoinServer{
		quit:         make(chan bool),
		shutdownChan: make(chan bool),
		btcdb:        btcdb,
		db: db,
		FundraiserHandler: &FundraiserHandler{db: bDb},
		SimpleSendHandler: &SimpleSendHandler{db: bDb},
	}
	parser := NewMsgParser(s.btcdb, s)
	s.msgParser = parser

	return s
}


// Process block takes care of sorting out transactions with exodus output.
// At this point it doesn't matter whether the Txs are valid or not. Validation
// is done at the proper handlers.
func (s *MastercoinServer) ProcessBlock(block *btcutil.Block) error {

	// Gather transactions from this block which had an exodus output
	txPack := &mscutil.TxPack{
		Txs:    mscutil.GetExodusTransactions(block),
		Time:   block.MsgBlock().Header.Timestamp.Unix(),
		Height: block.Height(),
	}

	// Update Exodus Vesting
	//


	// balance = ((1-(0.5 ** time_difference)) * 5631623576222 .round(8)


	if len(txPack.Txs) > 0 {
		serializedData, err := txPack.Serialize()
		if err != nil {
			return err
		}

		s.db.CreateTxPack(txPack.Height, serializedData)

		mscutil.Logger.Println("Mastercoin data found at block with height:", txPack.Height)

		// Queue the slice of transactions for further processing by the
		// message parser.
		_ = s.msgParser.ProcessTransactions(txPack)
	}

	return nil
}

// The inbound block handler
func (s *MastercoinServer) inboundBlockHandler() {
	mscutil.Logger.Println("Inbound block handler started")
out:
	for {
		select {
		case block := <-MastercoinBlockChannel:
			// Process the current block, check for Msc Txs
			err := s.ProcessBlock(block)
			if err != nil {
				mscutil.Logger.Println("Error processing block:", err.Error())
				continue
			}
		case <-s.quit:
			break out
		}
	}

	// Drain the block channel of any remaining blocks.
clean:
	for {
		select {
		case <-MastercoinBlockChannel:
			// Continue
		default:
			break clean
		}
	}
}

func (s *MastercoinServer) Stop() {
	// Close the channel and invoking the channel
	close(s.quit)

	s.db.Close()

	mscutil.Logger.Println("Stopped successfull")
}

func GetPath(p string) string {
	usr, _ := user.Current()

	return path.Join(usr.HomeDir, ".mastercoin", p)
}

func (s *MastercoinServer) Start(playback bool) {
	mscutil.Logger.Println("Started mastercoin server")
	// Start the message parser before the block handler input handler
	s.msgParser.Start()

	if playback{
		mscutil.Logger.Println("Starting playback")
		mscutil.Logger.Println("Removing database to start fresh")

		path := GetPath("bdb")
		err := os.RemoveAll(path)
		if err != nil {
			mscutil.Logger.Fatal(err)
		}


		iter := s.db.GetDb().NewIterator(nil)
		for iter.Next() {
			// Remember that the contents of the returned slice should not be modified, and
			// only valid until the next call to Next.
			value := iter.Value()

			newPack := &mscutil.TxPack{}
			newPack.Deserialize(value)

			mscutil.Logger.Println("\n####### Processing block with height:", newPack.Height, "#############")

			_ = s.msgParser.ProcessTransactions(newPack)
		}
		iter.Release()

		// This is where we compare fundraiser consensus
//		success := TestBalances(s.FundraiserHandler.db)

	}else{

		// Start the block inbound processing handler
		go s.inboundBlockHandler()
	}

}

func (s *MastercoinServer) WaitForShutdown() {
	<-s.shutdownChan
}

// TODO: When Simple Sends are all synced up; check rounding balance here
func CalculateMscDevCoins(time int64) float64 {
	timeDiff := float64(time - 1377993874) / 31556926
	powr := math.Pow(0.5, timeDiff)

	return (1 - powr ) * 56316.23576222
}

// Mastercoin main loop. Takes care of listening on the maistercoin block channel
// and calls the appropriate methods
func MastercoinMain(btcdb btcdb.Db) {
	// Main server instance
	server := NewMastercoinServer(btcdb)

	server.Start(false)
}
