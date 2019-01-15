package statuspage

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo"
)

func errorResponse(ctx echo.Context, httpStatus int, err error) error {
	response := ctx.Response()
	headers := response.Header()
	headers.Set("X-Result-Error", err.Error())

	return ctx.JSON(httpStatus, map[string]string{
		"status": "error",
		"error":  err.Error(),
	})
}

func okResponse(ctx echo.Context, data interface{}) error {
	return ctx.JSON(http.StatusOK, map[string]interface{}{
		"status": "ok",
		"result": data,
	})
}

type invalidFieldErr struct {
	fieldName string
}

func newInvalidFieldErr(fieldName string) invalidFieldErr {
	return invalidFieldErr{
		fieldName: fieldName,
	}
}
func (e invalidFieldErr) Error() string {
	return fmt.Sprintf(`Invalid field name: "%s"`, e.fieldName)
}
