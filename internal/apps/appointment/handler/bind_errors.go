package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"

	"github.com/go-playground/validator/v10"
)

func bindErrorDetail(err error) string {
	if err == nil {
		return ""
	}

	var validationErrors validator.ValidationErrors
	if errors.As(err, &validationErrors) && len(validationErrors) > 0 {
		return fieldErrorDetail(validationErrors[0])
	}

	var syntaxError *json.SyntaxError
	if errors.As(err, &syntaxError) {
		return "request body is not valid JSON"
	}

	var typeError *json.UnmarshalTypeError
	if errors.As(err, &typeError) {
		return fmt.Sprintf("%s must be %s", typeError.Field, jsonTypeName(typeError.Type))
	}

	var numberError *strconv.NumError
	if errors.As(err, &numberError) {
		return fmt.Sprintf("%q is not a valid number", numberError.Num)
	}

	if errors.Is(err, io.EOF) {
		return "request body is empty"
	}
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return "request body is not valid JSON"
	}

	return ""
}

func fieldErrorDetail(fieldError validator.FieldError) string {
	field := fieldError.Field()
	if _, path, found := strings.Cut(fieldError.Namespace(), "."); found {
		field = path
	}

	switch fieldError.Tag() {
	case "required":
		return field + " is required"
	case "email":
		return field + " must be a valid email"
	case "oneof":
		return fmt.Sprintf("%s must be one of: %s", field, fieldError.Param())
	case "min", "max":
		bound := "at least"
		if fieldError.Tag() == "max" {
			bound = "at most"
		}
		switch fieldError.Kind() {
		case reflect.String:
			return fmt.Sprintf("%s must be %s %s characters", field, bound, fieldError.Param())
		case reflect.Slice, reflect.Array, reflect.Map:
			return fmt.Sprintf("%s must have %s %s items", field, bound, fieldError.Param())
		default:
			return fmt.Sprintf("%s must be %s %s", field, bound, fieldError.Param())
		}
	default:
		return field + " is invalid"
	}
}

func jsonTypeName(goType reflect.Type) string {
	switch goType.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return "a number"
	case reflect.Bool:
		return "true or false"
	case reflect.Slice, reflect.Array:
		return "a list"
	case reflect.Struct, reflect.Map:
		return "an object"
	default:
		return "text"
	}
}
