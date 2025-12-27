package handlers

import (
	"zbor/web/components"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"
)

func Home(c echo.Context) error {
	return render(c, components.Home())
}

func render(c echo.Context, component templ.Component) error {
	return component.Render(c.Request().Context(), c.Response())
}
