package mscd

import (
	"github.com/conformal/btcscript"
	"github.com/conformal/btcutil"
	"github.com/mastercoin-MSC/mscutil"

	"log"
	"time"
)

// Bitcoin specific type checking
func isClassA(tx *btcutil.Tx) bool {
	mtx := tx.MsgTx()
	for _, txOut := range mtx.TxOut {
		_, scriptType := mscutil.GetAddrs(txOut.PkScript)
		if scriptType == btcscript.MultiSigTy {
			return false
		}
	}

	// If it wasn't multi sig it's class a
	return true
}

func isClassB(tx *btcutil.Tx) bool {
	return !isClassA(tx)
}

func getTxType(tx *btcutil.Tx) mscutil.TxType {
	if isClassA(tx) {
		return mscutil.ClassAType
	}

	return mscutil.ClassBType
}

type Msg struct {
	block *btcutil.Block
	raw   *btcutil.Tx
	msg   interface{}
}

const (
	msgChannelSize = 50
)

type MsgParser struct {
	// Server
	server *MastercoinServer
	// Inbound message channel
	messageChan chan *TxPack
	// Quit indicator
	quit chan bool

	// Public message channel. New outbound messages are dumped here
	outboundMsgChan chan *Msg
}

func NewMsgParser(server *MastercoinServer) *MsgParser {
	msgParser := &MsgParser{
		server:          server,
		messageChan:     make(chan *TxPack, msgChannelSize),
		quit:            make(chan bool),
		outboundMsgChan: make(chan *Msg, msgChannelSize),
	}

	return msgParser
}

// Parse transaction takes care of testing the tx for certain rules; class A/B checking
// Fundraiser checks and Dex messages as well as properly delegating the raw messages to
// the proper object which. After a full Msg has been assembled it will return the Msg
// or an error with a description. Whoever called this function should delegate the msg
// to the proper object handler.
func (mp *MsgParser) ParseTx(tx *btcutil.Tx, block *btcutil.Block) (msg *Msg, err error) {

	// Check if this is a Class A transaction
	if isClassA(tx) {
		log.Println("Got 'Class A' tx")

		// Create a simple send transaction
		simpleSend, err := mscutil.MakeClassASimpleSend(mscutil.GetAddrsClassA(tx))

		if simpleSend != nil {
			log.Println("Got Simple Send transaction")
			msg = &Msg{msg: simpleSend}
		}
	} else {
		log.Println("Got 'Class B' tx")

		// Parse addresses from the tx using class B rules
		out := mscutil.GetAddrsClassB(tx)
		// Receiver is first, data is second in the slice
		data := out[1]
		// Figure out the message type
		msgType := GetTypeFromAddress(data)
		switch msgType {
		case TxMsgTy:
			simpleSend, err := mscutil.MakeClassBSimpleSend(out)
		case DexMsgTy:
		default:
			log.Println("Unknown message type %d. FIXME or erroneus.\n", int(msgType))
		}
	}

	// If nothing has been found it is either a Exodus Kickstarter / Payment for accept DEx message.
	if msg == nil && err != nil {

		if block.Height() <= mscutil.FundraiserEndBlock {

			inputs := make(map[string]int64)

			// Collect the addresses and values for every input used for this transaction
			for _, txIn := range tx.MsgTx().TxIn {

				op := txIn.PreviousOutpoint
				hash := op.Hash
				index := op.Index
				transactions, err := mp.server.btcdb.FetchTxBySha(&hash)
				if err != nil {
					return nil, err
				}

				previousOutput := transactions[0].Tx.TxOut[index]

				// The largest contributor receives the Mastercoins, so add multiple address values together
				address, _ := mscutil.GetAddrs(previousOutput.PkScript)
				inputs[address[0]] += previousOutput.Value
			}

			log.Println("inputs:", inputs)

			// Decide which input has the most value so we know who 'won' this fundraiser transaction
			var highest int64
			var highestAddress string

			for k, v := range inputs {
				if v > highest {
					highest = v
					highestAddress = k
				}
			}

			var totalSpend int64
			for _, txOut := range tx.MsgTx().TxOut {
				totalSpend += txOut.Value
			}

			log.Println("Calcualted total spend:", totalSpend)

			fundraiserTx, err := mscutil.NewFundraiserTransaction(highestAddress, totalSpend, block.MsgBlock().Header.Timestamp.Unix())
			e = err

			msg = &Msg{msg: fundraiserTx}
		} else {
			//MakeDexMessage(outputs)
		}
	}

	return
}

func (mp *MsgParser) ProcessTransactions(txPack *TxPack) []*Msg {
	messages := make([]*Msg, len(txPack.txs))

	for _, tx := range txPack.txs {
		// TODO check if the txs exists in db otherwise skip (it might have happened that only a part of the block has been processed due to power failure)
		msg, err := mp.ParseTx(tx, txPack.block)
		if err != nil {
			log.Println(err)
		}

		log.Panic("DAAAMN SON")

		switch msg.msg.(type) {
		case mscutil.SimpleTransaction:
			// Process the simple send transaction (simple send transaction handler?)
			//mp.server.SimpleSendHandler.Process(msg.msg, err)
		case mscutil.FundraiserTransaction:
			//mp.server.FundraiserHandler.Process(msg.msg, err)
		}
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
