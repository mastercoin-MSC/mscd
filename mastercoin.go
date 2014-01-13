package mscd

import (
	_ "errors"
	_ "github.com/conformal/btcscript"
	"github.com/conformal/btcutil"
	_ "github.com/conformal/btcwire"
	"github.com/mastercoin-MSC/mscutil"
	"log"
)

var MastercoinBlockChannel chan *btcutil.Block

func QueueMessageForBlockChannel(block *btcutil.Block) {
	go func() {
		MastercoinBlockChannel <- block
	}()
}

type MastercoinServer struct {
	quit chan bool
}

func NewMastercoinServer() *MastercoinServer {
	MastercoinBlockChannel = make(chan *btcutil.Block, 1)

	return &MastercoinServer{}
}

func (s *MastercoinServer) ProcessBlock(block *btcutil.Block) error {
	mscutil.GetExodusTransactions(block)

	return nil
}

func (s *MastercoinServer) Stop() {
	s.quit <- true
}

// Mastercoin main loop. Takes care of listening on the maistercoin block channel
// and calls the appropriate methods
func MastercoinMain() {
	// Main server instance
	server := NewMastercoinServer()
out:
	for {
		select {
		case block := <-MastercoinBlockChannel:
			if block.Height() == 249497 {
				log.Panic("We got it bitch")
			}

			//err := server.ProcessBlock(block)
			//if err != nil {
			//  log.Println(err)
			//  continue
			//}
		case <-server.quit:
			break out
		}
	}
}
