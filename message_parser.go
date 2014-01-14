package mscd

import (
	"github.com/conformal/btcutil"
	"log"
	"time"
)

type Msg struct {
	raw *btcutil.Tx
	msg interface{}
}

const (
	msgChannelSize = 50
)

type MsgParser struct {
	// Inbound message channel
	messageChan chan []*btcutil.Tx
	// Quit indicator
	quit chan bool

	// Public message channel. New outbound messages are dumped here
	outboundMsgChan chan *Msg
}

func NewMsgParser() *MsgParser {
	msgParser := &MsgParser{
		messageChan:     make(chan []*btcutil.Tx, msgChannelSize),
		quit:            make(chan bool),
		outboundMsgChan: make(chan *Msg, msgChannelSize),
	}

	return msgParser
}

func (mp *MsgParser) QueueTransactions(txs []*btcutil.Tx) {
	mp.messageChan <- txs
}

func (mp *MsgParser) ParseTx(tx *btcutil.Tx) *Msg {
	// TODO
	// 1) Type check (class A/B tx, etc)
	// 2) Create appropriate message for the next handler
	return nil
}

func (mp *MsgParser) ProcessTransactions(txs []*btcutil.Tx) {
	//messages := make([]*Msg, len(txs))
	for _, tx := range txs {
		//messages[i] = mp.ParseTx(tx)
		mp.outboundMsgChan <- mp.ParseTx(tx)
	}

	//return messages
}

func (mp *MsgParser) transactionHandler() {
	ticker := time.NewTicker(10 * time.Second)
out:
	for {
		select {
		case txs := <-mp.messageChan:
			// Parse the transactions
			mp.ProcessTransactions(txs)
		case <-ticker.C:
			//log.Println("[MSGP]: PING")
		case <-mp.quit:
			break out
		}
	}
}

func (mp *MsgParser) Start() {
	go mp.transactionHandler()
}

func (mp *MsgParser) Stop() {
	log.Println("[MSP]: Shutdown")
}
