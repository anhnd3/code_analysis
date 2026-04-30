package service

import "example.com/single/internal/helper"

func Handle(input string) string {
	return stepOne(helper.Transform(input))
}

func stepOne(input string) string {
	return input
}
