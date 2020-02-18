package handlers

import (
	"errors"
	"fmt"
)

type Errors []error

func NewErrors() *Errors {
	return new(Errors)
}

func (errs *Errors) AddError(e error) { *errs = append(*errs, e) }

func (errs *Errors) Error() (err error) {
	msg := ""
	for _, e := range *errs {
		msg += fmt.Sprintln(e.Error())
	}

	return errors.New(msg)
}

func (errs *Errors) Empty() bool {
	return errs.Size() == 0
}

func (errs *Errors) Size() int {
	return len(*errs)
}
