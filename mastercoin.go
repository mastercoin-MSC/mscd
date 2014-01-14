package mscd

import (
	_ "errors"
	_ "github.com/conformal/btcscript"
	"github.com/conformal/btcutil"
	_ "github.com/conformal/btcwire"
	"github.com/mastercoin-MSC/mscutil"
	"log"
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
}

func NewMastercoinServer() *MastercoinServer {
	MastercoinBlockChannel = make(chan *btcutil.Block, blockChannelQueueLength)

	parser := NewMsgParser()

	return &MastercoinServer{
		quit:         make(chan bool),
		shutdownChan: make(chan bool),
		msgParser:    parser,
	}
}

// Process block takes care of sorting out transactions with exodus output.
// At this point it doesn't matter whether the Txs are valid or not. Validation
// is done at the proper handlers.
func (s *MastercoinServer) ProcessBlock(block *btcutil.Block) error {
	// Gather transactions from this block which had an exodus output
	txs := mscutil.GetExodusTransactions(block)
	// Queue the slice of transactions for further processing by the
	// message parser. The message will delegate to the appropriate channels
	s.msgParser.QueueTransactions(txs)

	return nil
}

// The inbound block handler
func (s *MastercoinServer) inboundBlockHandler() {
out:
	for {
		select {
		case block := <-MastercoinBlockChannel:
			if block.Height() == ExodusBlockHeight-1 {
				// TODO debugging hook for exodus

				// Block lower than exodus don't need parsing
				break
			}

			// Process the current block, check for Msc Txs
			err := s.ProcessBlock(block)
			if err != nil {
				log.Println(err)
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

	log.Println("[SERV]: Stopped successfull")
}

func (s *MastercoinServer) Start() {
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
func MastercoinMain() {
	// Main server instance
	server := NewMastercoinServer()

	server.Start()

	server.WaitForShutdown()
}
