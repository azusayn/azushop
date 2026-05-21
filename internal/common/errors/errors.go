package errors

type AzushopError struct {
	Code int
	Err  error
}

func (e *AzushopError) Error() string {
	return ""
}

func (e *AzushopError) Wrap(code int, err error) *AzushopError {
	return &AzushopError{
		Code: code,
		Err:  err,
	}
}

func (e *AzushopError) UnWrap() error {
	return nil
}

type ErrorCode int

const (
	ErrUserNotExist              ErrorCode = 10000
	ErrUsernameAlreadyExist      ErrorCode = 10001
	ErrInvalidUsernameOrPassword ErrorCode = 10002
)

type ErrorMessage string

const (
	MsgUserNotExist              ErrorMessage = "user not exists"
	MsgUsernameAlreadyExist      ErrorMessage = "username already exists"
	MsgInvalidUsernameOrPassword ErrorMessage = "invalid username or password"
)
