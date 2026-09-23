package inspect

// Fault is what assembling an inspector can fail with.
//
// It is not a failure channel. The inspector's own handlers cannot fail --
// which is why its Surface is generic in E and mounts inside any program's
// routes whatever that program fails with -- so this is only ever returned as
// a Go error from assembly, where a mistake in the surface is found at
// start-up.
type Fault struct {
	Op  string
	Err error
}

func (fault Fault) Error() string {
	if fault.Err == nil {
		return "inspect: " + fault.Op
	}
	return "inspect: " + fault.Op + ": " + fault.Err.Error()
}

func (fault Fault) Unwrap() error { return fault.Err }
