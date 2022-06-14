package client

import (
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/log"
	"github.com/hesusruiz/signers/types"
)

// QuorumClient provides access to quorum blockchain node.
type QuorumClient struct {
	wsClient *webSocketClient

	// To check we have actually shut down before returning
	shutdownChan chan struct{}
	shutdownWg   sync.WaitGroup
}

func newWebsocketQuorumClient(rawUrl string) (*QuorumClient, error) {
	quorumClient := &QuorumClient{
		shutdownChan: make(chan struct{}),
	}
	var err error
	log.Info("Connecting to Quorum WebSocket endpoint", "rawUrl", rawUrl)
	quorumClient.wsClient, err = newWebSocketClient(rawUrl)
	if err != nil {
		return nil, errors.New("connect Quorum WebSocket endpoint failed")
	}
	log.Info("Connected to WebSocket endpoint")

	return quorumClient, nil
}

func NewQuorumClient(rawUrl string) (*QuorumClient, error) {

	c, err := newWebsocketQuorumClient(rawUrl)
	if err != nil {
		return nil, err
	}

	// Start websocket receiver.
	go func() {
		c.shutdownWg.Add(1)
		c.wsClient.listen(c.shutdownChan)
		c.shutdownWg.Done()
	}()

	return c, nil
}

// Subscribe to chain head event.
func (qc *QuorumClient) SubscribeChainHead(ch chan<- types.RawHeader) error {
	return qc.wsClient.subscribeChainHead(ch)
}

// Execute customized rpc call.
func (qc *QuorumClient) RPCCall(result interface{}, method string, args ...interface{}) error {
	return qc.rpcCall(time.Second, result, method, args)
}

func (qc *QuorumClient) RPCCallWithTimeout(timeout time.Duration, result interface{}, method string, args ...interface{}) error {
	return qc.rpcCall(timeout, result, method, args)
}

// Execute customized rpc call.
func (qc *QuorumClient) rpcCall(timeout time.Duration, result interface{}, method string, args []interface{}) error {
	resultChan := make(chan *message, 1)
	err := qc.wsClient.sendRPCMsg(resultChan, method, args...)
	if err != nil {
		return err
	}

	rpcCallTimeout := time.NewTicker(timeout)
	defer rpcCallTimeout.Stop()
	select {
	case response := <-resultChan:
		if response == nil {
			return errors.New("nil rpc response")
		}
		log.Info("rpc call response", "response", string(response.Result))
		if response.Error != nil {
			return response.Error
		}
		if err := json.Unmarshal(response.Result, &result); err != nil {
			// if response.Result is not a JSON, assign to result directly
			reflect.ValueOf(result).Elem().Set(reflect.ValueOf(response.Result))
		}
		return nil
	case <-rpcCallTimeout.C:
		return errors.New("rpc call timeout")
	}
}

func (qc *QuorumClient) Stop() {
	close(qc.shutdownChan)
	if qc.wsClient.conn != nil {
		qc.wsClient.conn.Close()
	}
	qc.shutdownWg.Wait()
	log.Info("Quorum client stopped")
}
