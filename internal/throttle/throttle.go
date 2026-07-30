// Package throttle implements the documented contracts for this component. See Package, L0008 2.11, T4.
package throttle

import (
	"time"

	"ansm/internal/params"
)

// Delay follows the documented behavioral contract. See Delay, ThrottleExponentMax.
func Delay(n int) time.Duration {
	k := n
	if k > params.ThrottleExponentMax {
		k = params.ThrottleExponentMax
	}
	if k < 1 {
		k = 1
	}
	return params.ThrottleBase * (1 << uint(k-1))
}

// Plan follows the documented behavioral contract. See Plan.
type Plan struct {
	// Count follows the documented behavioral contract. See Count.
	Count int
	// Wait follows the documented behavioral contract. See Wait.
	Wait time.Duration
	// Throttled follows the documented behavioral contract. See Throttled.
	Throttled bool
	// RestartDelayed follows the documented behavioral contract. See RestartDelayed, L0008 5.9.
	RestartDelayed bool
}

// Next follows the documented behavioral contract. See Next, L0008 2.11, AppRestartDelay.
func Next(previous int, restartDelay time.Duration) Plan {
	count := previous + 1
	if previous == 0 {
		return Plan{Count: count}
	}

	throttled := Delay(count)
	wait := throttled
	if restartDelay > wait {
		wait = restartDelay
	}

	plan := Plan{Count: count, Wait: wait}
	if count == 1 && restartDelay > throttled {
		plan.RestartDelayed = true
	} else {
		plan.Throttled = true
	}
	return plan
}

// AfterHealthyStart follows the documented behavioral contract. See AfterHealthyStart, L0008 2.11, AppRestartDelay.
func AfterHealthyStart(restartDelay time.Duration) int {
	if restartDelay > 0 {
		return 1
	}
	return 0
}
