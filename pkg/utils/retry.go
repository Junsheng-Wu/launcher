package utils

import (
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	backoffSteps               = 10
	backoffFactor              = 1.25
	backoffDuration            = 5
	backoffJitter              = 1.0
	AnsiblePlanMaxRetryTimes   = 5
	AnsiblePlanExecuteInterval = 10 * time.Second
	RetryDeleteClusterInterval = 10 * time.Second
	DeleteClusterTimeout       = 2 * time.Minute
)

// Retry retries a given function with exponential backoff.

func Retry(maxRetries int, interval time.Duration, operation func() error) error {
	var err error
	for i := 0; i < maxRetries; i++ {
		if err = operation(); err == nil {
			return nil
		}

		time.Sleep(interval)
	}

	return err
}

// Poll tries a condition func until it returns true, an error, or the timeout
// is reached.
func Poll(interval, timeout time.Duration, condition wait.ConditionFunc) error {
	return wait.Poll(interval, timeout, condition)
}

// PollImmediate tries a condition func until it returns true, an error, or the timeout
// is reached.
func PollImmediate(interval, timeout time.Duration, condition wait.ConditionFunc) error {
	return wait.PollImmediate(interval, timeout, condition)
}
