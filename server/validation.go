package server

import (
	"reflect"
	"strings"
	"sync"

	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
)

var registerFieldNamesOnce sync.Once

func registerJSONFieldNames() {
	registerFieldNamesOnce.Do(func() {
		engine, ok := binding.Validator.Engine().(*validator.Validate)
		if !ok {
			return
		}

		engine.RegisterTagNameFunc(func(field reflect.StructField) string {
			for _, tag := range []string{"json", "uri", "form"} {
				name, _, _ := strings.Cut(field.Tag.Get(tag), ",")
				if name != "" && name != "-" {
					return name
				}
			}

			return field.Name
		})
	})
}
