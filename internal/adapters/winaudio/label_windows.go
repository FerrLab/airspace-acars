//go:build windows

// The ACARS plays cabin audio through the webview, so the audio session
// belongs to msedgewebview2.exe rather than to the ACARS. Windows labels the
// volume mixer from the session's display name, and WebView2 never sets one,
// so the mixer falls back to the process's file description and a pilot sees
// "Microsoft Edge WebView2" with the Edge icon next to a volume slider they
// are trying to use for the ACARS.
//
// Setting the process's AppUserModelID does not reach it: that names this
// process, while the session belongs to a child. The sessions have to be
// found through the Core Audio session enumerator and named from here.
package winaudio

import (
	"fmt"
	"os"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// COM identifiers for the Core Audio session APIs.
var (
	clsidMMDeviceEnumerator = windows.GUID{Data1: 0xBCDE0395, Data2: 0xE52F, Data3: 0x467C,
		Data4: [8]byte{0x8E, 0x3D, 0xC4, 0x57, 0x92, 0x91, 0x69, 0x2E}}
	iidIMMDeviceEnumerator = windows.GUID{Data1: 0xA95664D2, Data2: 0x9614, Data3: 0x4F35,
		Data4: [8]byte{0xA7, 0x46, 0xDE, 0x8D, 0xB6, 0x36, 0x17, 0xE6}}
	iidIAudioSessionManager2 = windows.GUID{Data1: 0x77AA99A0, Data2: 0x1BD6, Data3: 0x484F,
		Data4: [8]byte{0x8B, 0xC7, 0x2C, 0x65, 0x4C, 0x9A, 0x9B, 0x6F}}
	iidIAudioSessionControl2 = windows.GUID{Data1: 0xBFB7FF88, Data2: 0x7239, Data3: 0x4FC9,
		Data4: [8]byte{0x8F, 0xA2, 0x07, 0xC9, 0x50, 0xBE, 0x9C, 0x6D}}
)

// Vtable slots. There are no COM headers here, so each interface's method
// order is spelled out rather than trusted to memory.
const (
	// IUnknown: QueryInterface, AddRef, Release
	slotQueryInterface = 0
	slotRelease        = 2

	// IMMDeviceEnumerator: ..., EnumAudioEndpoints, GetDefaultAudioEndpoint
	slotGetDefaultAudioEndpoint = 4

	// IMMDevice: ..., Activate, OpenPropertyStore, GetId, GetState
	slotActivate = 3

	// IAudioSessionManager2: (IAudioSessionManager) GetAudioSessionControl,
	// GetSimpleAudioVolume, then GetSessionEnumerator
	slotGetSessionEnumerator = 5

	// IAudioSessionEnumerator: ..., GetCount, GetSession
	slotGetCount   = 3
	slotGetSession = 4

	// IAudioSessionControl: ..., GetState, GetDisplayName, SetDisplayName,
	// GetIconPath, SetIconPath, ...
	slotSetDisplayName = 5
	slotSetIconPath    = 7

	// IAudioSessionControl2: ..., GetSessionIdentifier,
	// GetSessionInstanceIdentifier, GetProcessId
	slotGetProcessId = 14
)

const (
	eRender   = 0 // EDataFlow
	eConsole  = 0 // ERole
	clsctxAll = 23
)

// x/sys/windows binds most of ole32 but not CoCreateInstance, so it is bound
// here rather than taking on a COM wrapper dependency for a single call.
var procCoCreateInstance = windows.NewLazySystemDLL("ole32.dll").NewProc("CoCreateInstance")

// call invokes a COM method by vtable slot. The first argument of every COM
// method is the interface pointer itself.
//
// Interface pointers are held as unsafe.Pointer rather than uintptr so that
// nothing here depends on a bare integer still being a valid address.
func call(obj unsafe.Pointer, slot int, args ...uintptr) uintptr {
	vtbl := *(**[32]uintptr)(obj)
	hr, _, _ := syscall.SyscallN(vtbl[slot], append([]uintptr{uintptr(obj)}, args...)...)
	return hr
}

func release(obj unsafe.Pointer) {
	if obj != nil {
		call(obj, slotRelease)
	}
}

func failed(hr uintptr) bool { return int32(hr) < 0 }

// Label names every audio session owned by this process or one of its
// descendants — which is where the webview's session lives. It returns how
// many sessions it named.
//
// A session only exists once something has actually played, and is destroyed
// again afterwards, so this is written to be called repeatedly: naming a
// session that is already named is harmless.
func Label(displayName, iconPath string) (labelled int, err error) {
	// COM apartment state is per-thread, so this must not be migrated.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// A failure in here is cosmetic. It must never reach the pilot.
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("naming the audio session panicked: %v", r)
		}
	}()

	// S_FALSE means the thread was already initialised, which is fine.
	if hr := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); hr != nil {
		if e, ok := hr.(syscall.Errno); !ok || int32(e) < 0 {
			return 0, fmt.Errorf("CoInitializeEx: %w", hr)
		}
	}
	defer windows.CoUninitialize()

	ours, err := processTree()
	if err != nil {
		return 0, err
	}

	var enumerator unsafe.Pointer
	if hr, _, _ := procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&clsidMMDeviceEnumerator)),
		0, // no aggregating outer object
		clsctxAll,
		uintptr(unsafe.Pointer(&iidIMMDeviceEnumerator)),
		uintptr(unsafe.Pointer(&enumerator)),
	); failed(hr) {
		return 0, fmt.Errorf("create device enumerator: 0x%08X", uint32(hr))
	}
	defer release(enumerator)

	var device unsafe.Pointer
	if hr := call(enumerator, slotGetDefaultAudioEndpoint, eRender, eConsole,
		uintptr(unsafe.Pointer(&device))); failed(hr) {
		// No output device at all: nothing to name, and not a fault.
		return 0, nil
	}
	defer release(device)

	var manager unsafe.Pointer
	if hr := call(device, slotActivate, uintptr(unsafe.Pointer(&iidIAudioSessionManager2)),
		clsctxAll, 0, uintptr(unsafe.Pointer(&manager))); failed(hr) {
		return 0, fmt.Errorf("activate session manager: 0x%08X", uint32(hr))
	}
	defer release(manager)

	var sessions unsafe.Pointer
	if hr := call(manager, slotGetSessionEnumerator, uintptr(unsafe.Pointer(&sessions))); failed(hr) {
		return 0, fmt.Errorf("session enumerator: 0x%08X", uint32(hr))
	}
	defer release(sessions)

	var count int32
	if hr := call(sessions, slotGetCount, uintptr(unsafe.Pointer(&count))); failed(hr) {
		return 0, fmt.Errorf("session count: 0x%08X", uint32(hr))
	}

	name, err := windows.UTF16PtrFromString(displayName)
	if err != nil {
		return 0, err
	}
	icon, err := windows.UTF16PtrFromString(iconPath)
	if err != nil {
		return 0, err
	}

	for i := int32(0); i < count; i++ {
		var control unsafe.Pointer
		if hr := call(sessions, slotGetSession, uintptr(i), uintptr(unsafe.Pointer(&control))); failed(hr) {
			continue
		}

		// The process id is only reachable through IAudioSessionControl2.
		var control2 unsafe.Pointer
		hr := call(control, slotQueryInterface, uintptr(unsafe.Pointer(&iidIAudioSessionControl2)),
			uintptr(unsafe.Pointer(&control2)))
		release(control)
		if failed(hr) {
			continue
		}

		var pid uint32
		if hr := call(control2, slotGetProcessId, uintptr(unsafe.Pointer(&pid))); failed(hr) {
			release(control2)
			continue
		}
		if !ours[pid] {
			release(control2)
			continue
		}

		if hr := call(control2, slotSetDisplayName, uintptr(unsafe.Pointer(name)), 0); !failed(hr) {
			labelled++
		}
		if iconPath != "" {
			call(control2, slotSetIconPath, uintptr(unsafe.Pointer(icon)), 0)
		}
		release(control2)
	}

	return labelled, nil
}

// processTree returns this process and every descendant of it. The webview
// runs as a child, so its audio session is found by process id rather than by
// trusting an executable name.
func processTree() (map[uint32]bool, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, fmt.Errorf("process snapshot: %w", err)
	}
	defer windows.CloseHandle(snapshot)

	parents := map[uint32]uint32{}
	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))

	for err := windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		parents[entry.ProcessID] = entry.ParentProcessID
	}

	self := uint32(os.Getpid())
	tree := map[uint32]bool{self: true}

	// Walk each process up towards a root, marking everything that reaches
	// us. The depth cap stops a recycled pid from making this a cycle.
	for pid := range parents {
		for cur, depth := pid, 0; cur != 0 && depth < 32; depth++ {
			if tree[cur] {
				tree[pid] = true
				break
			}
			next, ok := parents[cur]
			if !ok || next == cur {
				break
			}
			cur = next
		}
	}
	return tree, nil
}
