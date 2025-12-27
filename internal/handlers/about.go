package handlers

import (
	"zbor/web/components"

	"github.com/labstack/echo/v4"
)

func About(c echo.Context) error {
	return render(c, components.About())
}
