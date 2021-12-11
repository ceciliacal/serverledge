package api

import (
	"encoding/json"
	"github.com/grussorusso/serverledge/internal/config"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"github.com/grussorusso/serverledge/internal/functions"
	"github.com/grussorusso/serverledge/internal/scheduling"
	"github.com/labstack/echo/v4"
)

// GetFunctions handles a request to list the functions available in the system.
func GetFunctions(c echo.Context) error {
	// TODO
	return c.JSON(http.StatusOK, "No functions in the system.")
}

// InvokeFunction handles a function invocation request.
func InvokeFunction(c echo.Context) error {
	offloading := config.GetBool("offloading", false)
	funcName := c.Param("fun")
	function, ok := functions.GetFunction(funcName)
	if !ok {
		log.Printf("Dropping request for unknown function '%s'", funcName)
		return c.JSON(http.StatusNotFound, "")
	}
	params_map := make(map[string]string)
	err := json.NewDecoder(c.Request().Body).Decode(&params_map)
	if err != nil && err != io.EOF {
		log.Printf("Could not parse request params: %v", err)
		return err
	}

	r := &functions.Request{Fun: function, Params: params_map, Arrival: time.Now()}

	log.Printf("New request: %v", r)
	if result, err := scheduling.Schedule(r); err == nil {
		log.Printf("Request OK: %v", result.Result)
		return c.JSON(http.StatusOK, result.Result)
	} else if offloading {
		// offloading to handle missing resource status
		res, err := scheduling.Offload(r)
		defer res.Body.Close()
		if err == nil {
			body, _ := ioutil.ReadAll(res.Body)
			log.Printf("Offloading Request status OK: %s", string(body))
			return c.JSON(http.StatusOK, string(body))
		}
	}

	log.Printf("Failed invocation of %s: %v", function, err)
	return c.String(http.StatusServiceUnavailable, "")

}