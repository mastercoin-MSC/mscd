package mscd

import (
	"github.com/conformal/btcutil"
	"github.com/mastercoin-MSC/mscutil"
	"log"
	"time"
)

type Msg struct {
	block *btcutil.Block
	raw   *btcutil.Tx
	msg   interface{}
}

const (
	msgChannelSize = 50
)

type MsgParser struct {
	// Inbound message channel
	messageChan chan *TxPack
	// Quit indicator
	quit chan bool

	// Public message channel. New outbound messages are dumped here
	outboundMsgChan chan *Msg
}

func NewMsgParser() *MsgParser {
	msgParser := &MsgParser{
		messageChan:     make(chan *TxPack, msgChannelSize),
		quit:            make(chan bool),
		outboundMsgChan: make(chan *Msg, msgChannelSize),
	}

	return msgParser
}

func (mp *MsgParser) ParseTx(tx *btcutil.Tx) *Msg {
	// Extract the type from the transaction
	msgType := mscutil.GetType(tx)

	switch msgType {
	case mscutil.TxMsgTy:
		tx := mscutil.MakeTx(tx)
		log.Println(tx)
	}

	return nil
}

func (mp *MsgParser) ProcessTransactions(txPack *TxPack) []*Msg {
	messages := make([]*Msg, len(txPack.txs))
	for i, tx := range txPack.txs {
		messages[i] = mp.ParseTx(tx)
	}

	return messages
}

func (mp *MsgParser) QueueTransactions(txPack *TxPack) {
	mp.messageChan <- txPack
}

// Currently not in operation
func (mp *MsgParser) transactionHandler() {
	ticker := time.NewTicker(10 * time.Second)
out:
	for {
		select {
		case txPack := <-mp.messageChan:
			// Parse the transactions
			mp.ProcessTransactions(txPack)
		case <-ticker.C:
			//log.Println("[MSGP]: PING")
		case <-mp.quit:
			break out
		}
	}
}

func (mp *MsgParser) Start() {
	//go mp.transactionHandler()
}

func (mp *MsgParser) Stop() {
	log.Println("[MSP]: Shutdown")
}
