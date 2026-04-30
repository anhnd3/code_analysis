package handler

import "fmt"

type HandlerFunc func() string

func ReturnHandlerFunc() HandlerFunc {
	return func() string {
		return fmt.Sprintf("handled")
	}
}
