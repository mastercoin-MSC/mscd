package mscd

import (
	"github.com/conformal/btcutil"
)

type Tx struct {
	rawTx *btcutil.Tx
}

const (
	inboundQueueSize = 50
)

type TxHandler struct {
	inboundQueue chan *Tx
}

func NewTxHandler() *TxHandler {
	txHandler := &TxHandler{
		inboundQueue: make(chan *Tx, inboundQueueSize),
	}

	return txHandler
}
