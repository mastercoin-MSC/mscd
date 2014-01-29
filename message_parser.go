package mscd

import (
	"github.com/conformal/btcscript"
	"github.com/conformal/btcutil"
	"github.com/mastercoin-MSC/mscutil"
	"time"
	"bytes"
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
func (mp *MsgParser) ParseTx(tx *btcutil.Tx, height, time int64) (msg *Msg, err error) {
	sender,_ := mscutil.FindSender(tx.MsgTx().TxIn, mp.server.btcdb)

	// Check if this is a Class A transaction
	if isClassA(tx) {
		mscutil.Logger.Println("Got 'Class A' transaction with hash", tx.Sha())

		highestAddress, _ := mscutil.FindSender(tx.MsgTx().TxIn, mp.server.btcdb)

		// Create a simple send transaction
		simpleSend, e := mscutil.MakeClassASimpleSend(highestAddress, mscutil.GetAddrsClassA(tx))
		err = e

		if simpleSend != nil {
			mscutil.Logger.Println("Decoded Simple Send transaction:", simpleSend, simpleSend.Data)
			msg = &Msg{msg: simpleSend}
			mscutil.Logger.Fatal("SHUTDOWN")
		}
	} else {
		mscutil.Logger.Println("Got 'Class B' tx")

		// Parse addresses from the tx using class B rules
		plainTextKeys, receiver, err := mscutil.GetAddrsClassB(tx, sender)
		if err == nil {
			// Receiver is first, data is second in the slice
			data := plainTextKeys[0][1]
			// Figure out the message type
			msgType := mscutil.GetTypeFromAddress(string(data))
			switch msgType {
			case mscutil.TxMsgTy:
				simpleSend, e := mscutil.MakeClassBSimpleSend(plainTextKeys, receiver)
				err = e

				if simpleSend != nil {
					mscutil.Logger.Println("Got simple send class b transaction")
					msg = &Msg{msg: simpleSend}
				}
			case mscutil.DexMsgTy:
			default:
				mscutil.Logger.Println("Unknown message type %d. FIXME or erroneus.\n", int(msgType))
			}
		}
	}

	// If nothing has been found it is either a Exodus Kickstarter / Payment for accept DEx message.
	if msg == nil && err != nil {

		if height <= mscutil.FundraiserEndBlock {
			// Collect the addresses and values for every input used for this transaction
			highestAddress, _ := mscutil.FindSender(tx.MsgTx().TxIn, mp.server.btcdb)

			var totalSpend int64
			for _, txOut := range tx.MsgTx().TxOut {
				addr, _ := mscutil.GetAddrs(txOut.PkScript)
				if addr[0].Addr == mscutil.ExodusAddress {
					totalSpend += txOut.Value
				}
			}

			fundraiserTx, e := mscutil.NewFundraiserTransaction(highestAddress, totalSpend, time)
			err = e

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
		buff := new(bytes.Buffer)
		tx.MsgTx().Serialize(buff)

		// TODO check if the txs exists in db otherwise skip (it might have happened that only a part of the block has been processed due to power failure)
		msg, err := mp.ParseTx(tx, txPack.height, txPack.time)
		if err != nil {
			mscutil.Logger.Println(err)
		}

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
			//mscutil.Logger.Println("[MSGP]: PING")
		case <-mp.quit:
			break out
		}
	}
}

func (mp *MsgParser) Start() {
	//go mp.transactionHandler()
}

func (mp *MsgParser) Stop() {
	mscutil.Logger.Println("[MSP]: Shutdown")
}
