package service

import "example.com/single/internal/helper"

func Handle(input string) string {
	return stepOne(input)
}

func stepOne(input string) string {
	return helper.Transform(input)
}
