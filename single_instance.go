package main

import (
	"fmt"
	"net"
	"sync"
)

const singleInstanceAddr = "127.0.0.1:49876"

type SingleInstance struct {
	listener net.Listener
	mu       sync.Mutex
	onShow   func()
}

func NewSingleInstance() (*SingleInstance, error) {
	si := &SingleInstance{}

	listener, err := net.Listen("tcp", singleInstanceAddr)
	if err != nil {
		// Another instance is running — signal it to show its window
		conn, dialErr := net.Dial("tcp", singleInstanceAddr)
		if dialErr == nil {
			conn.Write([]byte("show"))
			conn.Close()
		}
		return nil, fmt.Errorf("another instance is already running")
	}

	si.listener = listener
	go si.listenLoop()
	return si, nil
}

func (si *SingleInstance) SetOnShow(fn func()) {
	si.mu.Lock()
	si.onShow = fn
	si.mu.Unlock()
}

func (si *SingleInstance) Close() {
	si.listener.Close()
}

func (si *SingleInstance) listenLoop() {
	for {
		conn, err := si.listener.Accept()
		if err != nil {
			return
		}
		buf := make([]byte, 4)
		conn.Read(buf)
		conn.Close()

		if string(buf) == "show" {
			si.mu.Lock()
			fn := si.onShow
			si.mu.Unlock()
			if fn != nil {
				fn()
			}
		}
	}
}
