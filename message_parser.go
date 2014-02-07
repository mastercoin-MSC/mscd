package mscd

import (
	"github.com/conformal/btcscript"
	"github.com/conformal/btcdb"
	"code.google.com/p/godec/dec"
	"strings"
	"github.com/conformal/btcutil"
	"math/big"
	"github.com/mastercoin-MSC/mscutil"
	"encoding/hex"
	"time"
	"fmt"
	"log"
	"bytes"
_	"strconv"
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
	server *MastercoinServer

	// DB
	btcdb btcdb.Db
	// Inbound message channel
	messageChan chan *mscutil.TxPack
	// Quit indicator
	quit chan bool

	// Public message channel. New outbound messages are dumped here
	outboundMsgChan chan *Msg
}

func NewMsgParser(btcdb btcdb.Db, server *MastercoinServer) *MsgParser {
	msgParser := &MsgParser{
		messageChan:     make(chan *mscutil.TxPack, msgChannelSize),
		quit:            make(chan bool),
		outboundMsgChan: make(chan *Msg, msgChannelSize),
		btcdb: btcdb,
		server: server,
	}

	return msgParser
}

// Parse transaction takes care of testing the tx for certain rules; class A/B checking
// Fundraiser checks and Dex messages as well as properly delegating the raw messages to
// the proper object which. After a full Msg has been assembled it will return the Msg
// or an error with a description. Whoever called this function should delegate the msg
// to the proper object handler.
func (mp *MsgParser) ParseTx(tx *btcutil.Tx, height, time int64) (msgs []*Msg, err error) {
	sender,_ := mscutil.FindSender(tx.MsgTx().TxIn, mp.btcdb)

	// Check if this is a Class A transaction
	if isClassA(tx) {
		mscutil.Logger.Println("Got 'Class A' transaction with hash", tx.Sha())

		// Create a simple send transaction
		simpleSend, e := mscutil.MakeClassASimpleSend(sender, mscutil.GetAddrsClassA(tx))
		err = e

		if simpleSend != nil {
			mscutil.Logger.Println("Decoded Simple Send transaction:", simpleSend, simpleSend.Data)
			msgs = append(msgs, &Msg{msg: simpleSend})
		}
	} else {
		mscutil.Logger.Println("Got 'Class B' tx with hash", tx.Sha())

		// Parse addresses from the tx using class B rules
		plainTextKeys, receiver, e := mscutil.GetAddrsClassB(tx, sender)
		err = e
		if err == nil {
			// Receiver is first, data is second in the slice

			// Let's change 00000014h to 22d
			data := plainTextKeys[0][2:10]
			f,_ := hex.DecodeString(data)
			messageType := int(f[3])

			switch messageType{
			case mscutil.TxMsgTy:
				simpleSend, e := mscutil.MakeClassBSimpleSend(plainTextKeys, receiver, sender)
				err = e

				if simpleSend != nil {
					// TODO: Do we want to add the sender and receiver to the simple send message?
					mscutil.Logger.Println("Got Simple Send Class B from",sender,"to",receiver)
					msgs = append(msgs, &Msg{msg: simpleSend})
				}
			default:
				err = fmt.Errorf("Unknown message type %d. Support for transaction type not implemented yet.\n",messageType)
				return nil, err
			}
		}
	}

	if height <= mscutil.FundraiserEndBlock {
		// Collect the addresses and values for every input used for this transaction
		highestAddress, _ := mscutil.FindSender(tx.MsgTx().TxIn, mp.btcdb)

		var totalSpend int64
		for _, txOut := range tx.MsgTx().TxOut {
			addr, _ := mscutil.GetAddrs(txOut.PkScript)
			if addr[0].Addr == mscutil.ExodusAddress {
				totalSpend += txOut.Value
			}
		}

		fundraiserTx, e := mscutil.NewFundraiserTransaction(highestAddress, totalSpend, time)
		err = e

		msgs = append(msgs, &Msg{msg: fundraiserTx})
	}

	// If nothing has been found it check for Payment for accept DEx message.
	if len(msgs) == 0 {
		// TODO:
	}

	return
}

type SimpleSendHandler struct {
	db *mscutil.LDBDatabase
}

type FundraiserHandler struct {
	db *mscutil.LDBDatabase
}

func (frh *FundraiserHandler) Process(msg *Msg, err error){
	pack := msg.msg.(*mscutil.FundraiserTransaction)

	// Get the current balance sheet map, where the key is the currency id and the value the amount
	sheet := frh.db.GetAccount(pack.Addr)

	
	str := strings.Split(dec.NewDecInt64(0).Mul(pack.Value, dec.NewDecInt64(1e8)).String(), ".")[0]
	res, _ := big.NewInt(0).SetString(str, 10)
	dec := res.Uint64()

	sheet[1] += dec
	sheet[2] += dec

	// Save the key back to the db
	frh.db.PutAccount(pack.Addr, sheet)
	sheet = frh.db.GetAccount(pack.Addr)
}
func (ssh *SimpleSendHandler) Process(msg *Msg, err error) {
	pack := msg.msg.(*mscutil.SimpleTransaction)

	sSheet := ssh.db.GetAccount(pack.Sender)
	rSheet := ssh.db.GetAccount(pack.Receiver)
	id := pack.Data.CurrencyId

	if sSheet[id] >= pack.Data.Amount{
		// Valid Transaction
	    rSheet[id] += pack.Data.Amount
	    sSheet[id] -= pack.Data.Amount

	    ssh.db.PutAccount(pack.Sender, sSheet)
	    ssh.db.PutAccount(pack.Receiver, rSheet)

	      mscutil.Logger.Println("Send", pack.Data.Amount, "From", pack.Sender, "To", pack.Receiver)
	} else{
	      // Invalid Transactions
	      mscutil.Logger.Println("Not enough balance, transaction invalid")
	}


}

func (mp *MsgParser) ProcessTransactions(txPack *mscutil.TxPack) []*Msg {

	//TODO: Build a way to calculate the difference of MSC vested between this block and last and update the balance accordingly
	//deVcoins := CalculateMscDevCoins(txPack.Time)

	messages := make([]*Msg, len(txPack.Txs))

	for _, tx := range txPack.Txs {
		buff := new(bytes.Buffer)
		tx.MsgTx().Serialize(buff)

		msgs, err := mp.ParseTx(tx, txPack.Height, txPack.Time)

		for _, msg := range msgs {
			if err != nil {
				mscutil.Logger.Println("Message could not be parsed:", err)
			}else{
				switch msg.msg.(type) {
				case *mscutil.SimpleTransaction:
					// Process the simple send transaction (simple send transaction handler?)
					//mp.server.SimpleSendHandler.Process(msg, err)
				case *mscutil.FundraiserTransaction:
					// TODO: UNCOMMENT ME
					mp.server.FundraiserHandler.Process(msg, err)
				default:
					log.Fatal("Unknown message type", msg.msg)
				}
			}
		}
	}

	return messages
}

func (mp *MsgParser) QueueTransactions(txPack *mscutil.TxPack) {
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
