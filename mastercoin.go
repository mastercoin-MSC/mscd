package mscd

import (
	_ "errors"
	"github.com/conformal/btcdb"
	_ "github.com/conformal/btcscript"
	"github.com/conformal/btcutil"
	_ "github.com/conformal/btcwire"
	"github.com/mastercoin-MSC/mscutil"
	"fmt"
	"bytes"
	_ "encoding/gob"
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
}

func NewMastercoinServer(btcdb btcdb.Db) *MastercoinServer {
	mscutil.Logger.Println("Creating mastercoin server")
	MastercoinBlockChannel = make(chan *btcutil.Block, blockChannelQueueLength)

	s := &MastercoinServer{
		quit:         make(chan bool),
		shutdownChan: make(chan bool),
		btcdb:        btcdb,
	}
	parser := NewMsgParser(s)
	s.msgParser = parser

	return s
}

type TxPack struct {
	time   int64
	height int64
	txs    []*btcutil.Tx
}

func (pack *TxPack) MarshalBinary() ([]byte, error) {
	buff := new(bytes.Buffer)

	// Length of the transactions slice so the unmarshaler can allocate a large enough slice
	length := len(pack.txs)
	fmt.Fprintln(buff, pack.time, pack.height, length)
	// Write each tx
	for _, tx := range pack.txs {
		var data bytes.Buffer
		tx.MsgTx().Serialize(&data)
		buff.Write(data.Bytes())

		fmt.Fprintf(buff, "\n")
	}

	return buff.Bytes(), nil
}

func (pack *TxPack) UnmarshalBinary(data []byte) error {
	buff := bytes.NewBuffer(data)

	var length int
	_, err := fmt.Fscanln(buff, &pack.time, &pack.height, &length)

	pack.txs = make([]*btcutil.Tx, length)
	for i := 0; i < length; i++ {
		var b []byte
		_, err = fmt.Fscanln(buff, &b)
		if err == nil {
			pack.txs[i], err = btcutil.NewTxFromBytes(b)
			fmt.Println("err from btc")
		}
	}

	return err
}

// Process block takes care of sorting out transactions with exodus output.
// At this point it doesn't matter whether the Txs are valid or not. Validation
// is done at the proper handlers.
func (s *MastercoinServer) ProcessBlock(block *btcutil.Block) error {
	// Gather transactions from this block which had an exodus output
	txPack := &TxPack{
		txs:    mscutil.GetExodusTransactions(block),
		time:   block.MsgBlock().Header.Timestamp.Unix(),
		height: block.Height(),
	}
	if len(txPack.txs) > 0 {

		/*
		TODO: We want to start persisting raw data here
		var buffer bytes.Buffer
		enc := gob.NewEncoder(&buffer)
		err := enc.Encode(txPack)
		if err != nil {
			mscutil.Logger.Fatal(err)
		}

		dec := gob.NewDecoder(&buffer)
		var pack TxPack
		err = dec.Decode(&pack)
		if err != nil {
			mscutil.Logger.Fatal(err)
		}
		*/

		// Queue the slice of transactions for further processing by the
		// message parser.
		_ = s.msgParser.ProcessTransactions(txPack)
		// mscutil.Logger.Println(messages)
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
			if block.Height() > ExodusBlockHeight {
				// TODO debugging hook for exodus

			} else {
				// Block lower than exodus don't need parsing
				break
			}

			// Process the current block, check for Msc Txs
			err := s.ProcessBlock(block)
			if err != nil {
				mscutil.Logger.Println(err)
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

	mscutil.Logger.Println("Stopped successfull")
}

func (s *MastercoinServer) Start() {
	mscutil.Logger.Println("Started mastercoin server")
	// Start the message parser before the block handler input handler
	s.msgParser.Start()

	// Start the block inbound processing handler
	go s.inboundBlockHandler()

}

func (s *MastercoinServer) WaitForShutdown() {
	<-s.shutdownChan
}

// Mastercoin main loop. Takes care of listening on the maistercoin block channel
// and calls the appropriate methods
func MastercoinMain(btcdb btcdb.Db) {
	// Main server instance
	server := NewMastercoinServer(btcdb)

	server.Start()

	server.WaitForShutdown()
}
