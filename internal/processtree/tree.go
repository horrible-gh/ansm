// Package processtree contains the PID-reuse checks shared by Windows process
// traversal and its platform-independent tests.
package processtree

// IsRealChild rejects a process whose parent PID matches only because Windows
// reused a PID outside the lifetime of the candidate parent.
func IsRealChild(parentPID, entryParentPID uint32, parentCreated, parentExited, childCreated uint64) bool {
	if parentPID == 0 || entryParentPID != parentPID {
		return false
	}
	if parentCreated == 0 || parentExited == 0 || childCreated == 0 {
		return false
	}
	return childCreated >= parentCreated && childCreated <= parentExited
}
