package simconnect

import (
	"fmt"
	"reflect"
	"syscall"
	"unsafe"
)

var (
	procOpen                        *syscall.LazyProc
	procClose                       *syscall.LazyProc
	procAddToDataDefinition         *syscall.LazyProc
	procGetNextDispatch             *syscall.LazyProc
	procRequestDataOnSimObjectType  *syscall.LazyProc
)

// SimConnect wraps a handle to the SimConnect session.
type SimConnect struct {
	handle      uintptr
	DefineMap   map[string]DWORD
	LastEventID DWORD
}

// New opens a SimConnect session. The dllPath must point to a valid
// 64-bit SimConnect.dll.
func New(name string, dllPath string) (*SimConnect, error) {
	if procOpen == nil {
		mod := syscall.NewLazyDLL(dllPath)
		if err := mod.Load(); err != nil {
			return nil, fmt.Errorf("load SimConnect.dll: %w", err)
		}

		procOpen = mod.NewProc("SimConnect_Open")
		procClose = mod.NewProc("SimConnect_Close")
		procAddToDataDefinition = mod.NewProc("SimConnect_AddToDataDefinition")
		procGetNextDispatch = mod.NewProc("SimConnect_GetNextDispatch")
		procRequestDataOnSimObjectType = mod.NewProc("SimConnect_RequestDataOnSimObjectType")
	}

	s := &SimConnect{
		DefineMap:   map[string]DWORD{"_last": 0},
		LastEventID: 0,
	}

	args := []uintptr{
		uintptr(unsafe.Pointer(&s.handle)),
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(name))),
		0, 0, 0, 0,
	}
	r1, _, err := procOpen.Call(args...)
	if int32(r1) < 0 {
		return nil, fmt.Errorf("SimConnect_Open: %d %s", int32(r1), err)
	}

	return s, nil
}

// Close closes the SimConnect session.
func (s *SimConnect) Close() error {
	r1, _, err := procClose.Call(uintptr(s.handle))
	if int32(r1) < 0 {
		return fmt.Errorf("SimConnect_Close: %d %s", int32(r1), err)
	}
	return nil
}

// AddToDataDefinition adds a variable to a data definition.
func (s *SimConnect) AddToDataDefinition(defineID DWORD, name, unit string, dataType DWORD) error {
	r1, _, err := procAddToDataDefinition.Call(
		uintptr(s.handle),
		uintptr(defineID),
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(name))),
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(unit))),
		uintptr(dataType),
		0xffffffff, // epsilon — UNUSED
		0xffffffff, // datum id — UNUSED
	)
	if int32(r1) < 0 {
		return fmt.Errorf("SimConnect_AddToDataDefinition %s: %d %s", name, int32(r1), err)
	}
	return nil
}

// RegisterDataDefinition registers a struct's tagged fields as a data definition.
// The struct must be passed by pointer. Fields must have "name" and "unit" tags.
// The first field (embedded header) is skipped.
func (s *SimConnect) RegisterDataDefinition(a interface{}) error {
	defineID := s.GetDefineID(a)

	v := reflect.ValueOf(a).Elem()
	for j := 1; j < v.NumField(); j++ {
		fieldName := v.Type().Field(j).Name
		nameTag, _ := v.Type().Field(j).Tag.Lookup("name")
		unitTag, _ := v.Type().Field(j).Tag.Lookup("unit")

		fieldType := v.Field(j).Kind().String()
		if fieldType == "array" {
			fieldType = fmt.Sprintf("[%d]byte", v.Field(j).Type().Len())
		}

		if nameTag == "" {
			return fmt.Errorf("%s name tag not found", fieldName)
		}

		dataType, err := derefDataType(fieldType)
		if err != nil {
			return err
		}

		if err := s.AddToDataDefinition(defineID, nameTag, unitTag, dataType); err != nil {
			return err
		}
	}

	return nil
}

// GetDefineID returns the define ID for the given struct type,
// assigning a new one if not yet registered.
func (s *SimConnect) GetDefineID(a interface{}) DWORD {
	structName := reflect.TypeOf(a).Elem().Name()
	id, ok := s.DefineMap[structName]
	if !ok {
		id = s.DefineMap["_last"]
		s.DefineMap[structName] = id
		s.DefineMap["_last"] = id + 1
	}
	return id
}

// RequestDataOnSimObjectType requests data for all objects of a given type.
func (s *SimConnect) RequestDataOnSimObjectType(requestID, defineID, radius, simobjectType DWORD) error {
	args := []uintptr{
		uintptr(s.handle),
		uintptr(requestID),
		uintptr(defineID),
		uintptr(radius),
		uintptr(simobjectType),
	}
	r1, _, err := procRequestDataOnSimObjectType.Call(args...)
	if int32(r1) < 0 {
		return fmt.Errorf("SimConnect_RequestDataOnSimObjectType: %d %s", int32(r1), err)
	}
	return nil
}

// GetNextDispatch retrieves the next dispatch message from SimConnect.
// Returns (pointer to data, HRESULT, error). When r1 < 0 there is no message.
func (s *SimConnect) GetNextDispatch() (unsafe.Pointer, int32, error) {
	var ppData unsafe.Pointer
	var ppDataLength DWORD

	r1, _, err := procGetNextDispatch.Call(
		uintptr(s.handle),
		uintptr(unsafe.Pointer(&ppData)),
		uintptr(unsafe.Pointer(&ppDataLength)),
	)

	return ppData, int32(r1), err
}

// derefDataType maps Go type strings to SimConnect data types.
func derefDataType(fieldType string) (DWORD, error) {
	switch fieldType {
	case "int32":
		return DATATYPE_INT32, nil
	case "int64":
		return DATATYPE_INT64, nil
	case "float32":
		return DATATYPE_FLOAT32, nil
	case "float64":
		return DATATYPE_FLOAT64, nil
	case "[8]byte":
		return DATATYPE_STRING8, nil
	case "[32]byte":
		return DATATYPE_STRING32, nil
	case "[64]byte":
		return DATATYPE_STRING64, nil
	case "[128]byte":
		return DATATYPE_STRING128, nil
	case "[256]byte":
		return DATATYPE_STRING256, nil
	case "[260]byte":
		return DATATYPE_STRING260, nil
	default:
		return 0, fmt.Errorf("unsupported data type: %s", fieldType)
	}
}
