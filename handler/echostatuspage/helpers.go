package echostatuspage

import (
	"net/http"

	"github.com/labstack/echo"
)

// errorResponse is a helper function to return an error
func errorResponse(ctx echo.Context, httpStatus int, err error) error {
	response := ctx.Response()
	headers := response.Header()
	headers.Set("X-Result-Error", err.Error())

	return ctx.JSON(httpStatus, map[string]string{
		"status": "error",
		"error":  err.Error(),
	})
}

// okResponse is a helper function to return some data if there was no errors
func okResponse(ctx echo.Context, data interface{}) error {
	return ctx.JSON(http.StatusOK, map[string]interface{}{
		"status": "ok",
		"result": data,
	})
}
