package main

import (
	"context"
	"math/big"
	"strings"
	"time"

	"github.com/openrelayxyz/plugeth-utils/core"
	"github.com/openrelayxyz/plugeth-utils/restricted"
	"gopkg.in/urfave/cli.v1"
)

var (
	pl      core.PluginLoader
	backend restricted.Backend
	log     core.Logger
	events  core.Feed
)

var (
	httpApiFlagName = "http.api"
	wsApiFlagName   = "ws.api"
)

func Initialize(ctx *cli.Context, loader core.PluginLoader, logger core.Logger) {
	log = logger
	pl = loader
	events = pl.GetFeed()
	v := ctx.GlobalString(httpApiFlagName)
	if v == "" {
		ctx.GlobalSet(wsApiFlagName, "eth,net,web3,plugeth")
	} else if !strings.Contains(v, "plugeth") {
		ctx.GlobalSet(wsApiFlagName, v+",plugeth")
	}
	log.Info("Loaded Block Tracer")
}

type TracerResult struct {
	CallStack []CallStack
}

type CallStack struct {
	Type      string       `json:"type"`
	From      core.Address `json:"from"`
	To        core.Address `json:"to"`
	Value     *big.Int     `json:"value,omitempty"`
	Gas       uint64       `json:"gas"`
	GasUsed   uint64       `json:"gasUsed"`
	Input     []byte       `json:"input"`
	Output    []byte       `json:"output"`
	Time      string       `json:"time,omitempty"`
	Calls     []CallStack  `json:"calls,omitempty"` //may need a pointer
	Error     string       `json:"error,omitempty"`
	startTime time.Time
	outOff    uint64
	outLen    uint64
	gasIn     uint64
	gasCost   uint64
}

func (t *TracerResult) TraceBlock(ctx context.Context) (<-chan CallStack, error) {
	subch := make(chan CallStack, 1000)
	rtrnch := make(chan CallStack, 1000)
	go func() {
		log.Info("Subscription Block Tracer setup")
		sub := events.Subscribe(subch)
		for {
			select {
			case <-ctx.Done():
				sub.Unsubscribe()
				close(subch)
				close(rtrnch)
				return
			case t := <-subch:
				rtrnch <- t
			case <-sub.Err():
				sub.Unsubscribe()
				close(subch)
				close(rtrnch)
				return
			}
		}
	}()
	return rtrnch, nil
}

func GetLiveTracer(core.Hash, core.StateDB) core.BlockTracer {
	return &TracerResult{}
}

func (r *TracerResult) PreProcessBlock(hash core.Hash, number uint64, encoded []byte) {
}

func (r *TracerResult) PreProcessTransaction(tx core.Hash, block core.Hash, i int) {
}

func (r *TracerResult) BlockProcessingError(tx core.Hash, block core.Hash, err error) {
}

func (r *TracerResult) PostProcessTransaction(tx core.Hash, block core.Hash, i int, receipt []byte) {
}

func (r *TracerResult) PostProcessBlock(block core.Hash) {
}

func (r *TracerResult) CaptureStart(from core.Address, to core.Address, create bool, input []byte, gas uint64, value *big.Int) {
	r.CallStack = []CallStack{}
}

func (r *TracerResult) CaptureState(pc uint64, op core.OpCode, gas, cost uint64, scope core.ScopeContext, rData []byte, depth int, err error) {
}

func (r *TracerResult) CaptureFault(pc uint64, op core.OpCode, gas, cost uint64, scope core.ScopeContext, depth int, err error) {
}

func (r *TracerResult) CaptureEnd(output []byte, gasUsed uint64, t time.Duration, err error) {
	r.CallStack = append(r.CallStack, CallStack{
		GasUsed: gasUsed,
		Output:  output,
		Time:    t.String(),
		Calls:   []CallStack{},
	})
	log.Info("inside of capture end")
	if len(r.CallStack) > 0 {
		events.Send(r.CallStack[0])
	}
}

// type:    ctx.type,
// 			from:    toHex(ctx.from),
// 			to:      toHex(ctx.to),
// 			value:   '0x' + ctx.value.toString(16),
// 			gas:     '0x' + bigInt(ctx.gas).toString(16),
// 			gasUsed: '0x' + bigInt(ctx.gasUsed).toString(16),
// 			input:   toHex(ctx.input),{"method":"plugeth_subscribe", "params":["trace"],"id":7}
// 			output:  toHex(ctx.output),

func (r *TracerResult) CaptureEnter(typ core.OpCode, from core.Address, to core.Address, input []byte, gas uint64, value *big.Int) {
	r.CallStack = append(r.CallStack, CallStack{
		Type:  restricted.OpCode(typ).String(),
		From:  from,
		To:    to,
		Input: input,
		Gas:   gas,
		Calls: []CallStack{},
	})
}

func (r *TracerResult) CaptureExit(output []byte, gasUsed uint64, err error) {
	if len(r.CallStack) > 1 {
		log.Info("inside of if", "CallStack", len(r.CallStack))
		returnCall := r.CallStack[len(r.CallStack)-1]
		log.Info("Indexed callstack", "Indexed", len(r.CallStack[len(r.CallStack)-2].Calls))
		r.CallStack[len(r.CallStack)-2].Calls = append(r.CallStack[len(r.CallStack)-2].Calls, returnCall)
		r.CallStack = r.CallStack[:len(r.CallStack)-1]
	}
}

func (r *TracerResult) Result() (interface{}, error) {
	return "", nil
}

func GetAPIs(node core.Node, backend core.Backend) []core.API {
	defer log.Info("APIs Initialized")
	return []core.API{
		{
			Namespace: "plugeth",
			Version:   "1.0",
			Service:   &TracerResult{},
			Public:    true,
		},
	}
}