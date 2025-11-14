package app

type exitError struct {
	message string
}

func (e exitError) Error() string {
	return e.message
}

func newExitError(msg string) error {
	return exitError{message: msg}
}
