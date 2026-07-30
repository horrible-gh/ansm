// Package control implements the documented contracts for this component. See Package, P0007 1.2, L0008 2.18.
package control

// Code follows the documented behavioral contract. See Code, SCM.
type Code uint32

const (
	// Start follows the documented behavioral contract. See Start, Windows.
	Start Code = 0
	// Stop follows the documented behavioral contract. See Stop.
	Stop Code = 1
	// Pause follows the documented behavioral contract. See Pause.
	Pause Code = 2
	// Continue follows the documented behavioral contract. See Continue.
	Continue Code = 3
	// Interrogate follows the documented behavioral contract. See Interrogate.
	Interrogate Code = 4
	// Shutdown follows the documented behavioral contract. See Shutdown, Stop.
	Shutdown Code = 5
	// PowerEvent follows the documented behavioral contract. See PowerEvent.
	PowerEvent Code = 13
	// Rotate follows the documented behavioral contract. See Rotate.
	Rotate Code = 128
)

// This section follows the documented behavioral contract. See PowerEvent, L0008 4.2.
const (
	// This section follows the documented behavioral contract. See PBTAPMPowerStatusChange, Power, Change.
	PBTAPMPowerStatusChange = 10
	// This section follows the documented behavioral contract. See PBTAPMResumeAutomatic, Power, Resume.
	PBTAPMResumeAutomatic = 18
)

// Name follows the documented behavioral contract. See Name, NSSM_TRIGGER, NSSM_LAST_CONTROL.
func (c Code) Name() string {
	switch c {
	case Start:
		return "START"
	case Stop:
		return "STOP"
	case Pause:
		return "PAUSE"
	case Continue:
		return "CONTINUE"
	case Interrogate:
		return "INTERROGATE"
	case Shutdown:
		return "SHUTDOWN"
	case PowerEvent:
		return "POWEREVENT"
	case Rotate:
		return "ROTATE"
	default:
		return ""
	}
}

// State follows the documented behavioral contract. See State, P0007 1.3.
type State uint32

const (
	Stopped         State = 1
	StartPending    State = 2
	StopPending     State = 3
	Running         State = 4
	ContinuePending State = 5
	PausePending    State = 6
	Paused          State = 7
)

// String follows the documented behavioral contract. See String.
func (s State) String() string {
	switch s {
	case Stopped:
		return "SERVICE_STOPPED"
	case StartPending:
		return "SERVICE_START_PENDING"
	case StopPending:
		return "SERVICE_STOP_PENDING"
	case Running:
		return "SERVICE_RUNNING"
	case ContinuePending:
		return "SERVICE_CONTINUE_PENDING"
	case PausePending:
		return "SERVICE_PAUSE_PENDING"
	case Paused:
		return "SERVICE_PAUSED"
	default:
		return ""
	}
}

// Verdict follows the documented behavioral contract. See Verdict, Classify.
type Verdict int

const (
	// Desired follows the documented behavioral contract. See Desired.
	Desired Verdict = 0
	// Expected follows the documented behavioral contract. See Expected.
	Expected Verdict = 1
	// Unexpected follows the documented behavioral contract. See Unexpected.
	Unexpected Verdict = -1
)

// Classify follows the documented behavioral contract. See Classify, L0008 2.18, Interrogate, Rotate, Desired.
func Classify(c Code, s State) Verdict {
	switch c {
	case Start:
		switch s {
		case Running:
			return Desired
		case StartPending:
			return Expected
		}
	case Stop, Shutdown:
		switch s {
		case Stopped:
			return Desired
		case Running, StopPending:
			return Expected
		}
	case Pause:
		switch s {
		case Paused:
			return Desired
		case PausePending:
			return Expected
		}
	case Continue:
		switch s {
		case Running:
			return Desired
		case ContinuePending:
			return Expected
		}
	case Interrogate, Rotate:
		return Desired
	}
	return Unexpected
}
