//go:build windows

package main

import _ "embed"

//go:embed SimConnect.dll
var embeddedSimConnectDLL []byte
