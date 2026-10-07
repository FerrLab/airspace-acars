//go:build windows

package simconnect

// DWORD is the SimConnect unsigned 32-bit integer type.
type DWORD uint32

const (
	DATATYPE_INVALID      DWORD = iota
	DATATYPE_INT32              // 1
	DATATYPE_INT64              // 2
	DATATYPE_FLOAT32            // 3
	DATATYPE_FLOAT64            // 4
	DATATYPE_STRING8            // 5
	DATATYPE_STRING32           // 6
	DATATYPE_STRING64           // 7
	DATATYPE_STRING128          // 8
	DATATYPE_STRING256          // 9
	DATATYPE_STRING260          // 10
	DATATYPE_STRINGV            // 11
	DATATYPE_INITPOSITION       // 12
	DATATYPE_MARKERSTATE        // 13
	DATATYPE_WAYPOINT           // 14
	DATATYPE_LATLONALT          // 15
	DATATYPE_XYZ                // 16
	DATATYPE_MAX                // 17
)

const (
	RECV_ID_NULL DWORD = iota
	RECV_ID_EXCEPTION
	RECV_ID_OPEN
	RECV_ID_QUIT
	RECV_ID_EVENT
	RECV_ID_EVENT_OBJECT_ADDREMOVE
	RECV_ID_EVENT_FILENAME
	RECV_ID_EVENT_FRAME
	RECV_ID_SIMOBJECT_DATA
	RECV_ID_SIMOBJECT_DATA_BYTYPE
	RECV_ID_WEATHER_OBSERVATION
	RECV_ID_CLOUD_STATE
	RECV_ID_ASSIGNED_OBJECT_ID
	RECV_ID_RESERVED_KEY
	RECV_ID_CUSTOM_ACTION
	RECV_ID_SYSTEM_STATE
	RECV_ID_CLIENT_DATA
	RECV_ID_EVENT_WEATHER_MODE
	RECV_ID_AIRPORT_LIST // 18
	RECV_ID_VOR_LIST
	RECV_ID_NDB_LIST
	RECV_ID_WAYPOINT_LIST
	RECV_ID_EVENT_MULTIPLAYER_SERVER_STARTED
	RECV_ID_EVENT_MULTIPLAYER_CLIENT_STARTED
	RECV_ID_EVENT_MULTIPLAYER_SESSION_ENDED
	RECV_ID_EVENT_RACE_END
	RECV_ID_EVENT_RACE_LAP
	RECV_ID_PICK
	RECV_ID_EVENT_EX1
	RECV_ID_FACILITY_DATA     // 29
	RECV_ID_FACILITY_DATA_END // 30
)

// FACILITY_LIST_TYPE_AIRPORT selects airports in SimConnect_RequestFacilitiesList_EX1.
const FACILITY_LIST_TYPE_AIRPORT DWORD = 0

// RecvFacilityDataEnd closes the reply to SimConnect_RequestFacilityData.
type RecvFacilityDataEnd struct {
	Recv
	RequestID DWORD
}

const (
	SIMOBJECT_TYPE_USER DWORD = iota
	SIMOBJECT_TYPE_ALL
	SIMOBJECT_TYPE_AIRCRAFT
	SIMOBJECT_TYPE_HELICOPTER
	SIMOBJECT_TYPE_BOAT
	SIMOBJECT_TYPE_GROUND
)

// Recv is the base header for all SimConnect dispatch messages.
type Recv struct {
	Size    DWORD
	Version DWORD
	ID      DWORD
}

// RecvSimobjectData is the header for RECV_ID_SIMOBJECT_DATA dispatches.
type RecvSimobjectData struct {
	Recv
	RequestID   DWORD
	ObjectID    DWORD
	DefineID    DWORD
	Flags       DWORD
	EntryNumber DWORD
	OutOf       DWORD
	DefineCount DWORD
}

// RecvSimobjectDataByType is the header for RECV_ID_SIMOBJECT_DATA_BYTYPE.
type RecvSimobjectDataByType struct {
	RecvSimobjectData
}

// RecvException is sent when SimConnect encounters an error.
type RecvException struct {
	Recv
	Exception DWORD
	SendID    DWORD
	Index     DWORD
}
