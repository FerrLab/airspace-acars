//go:build !windows

package main

func NewSimConnectAdapter() SimConnector {
	return nil
}
