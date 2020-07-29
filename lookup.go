package main

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"reflect"
)

type TagsNotSupportedError struct {
	msg string
}

func (e *TagsNotSupportedError) Error() string {
	return fmt.Sprint(e.msg, " tags not supported")
}

type NotImplementedError struct {
	msg string
}

func (e *NotImplementedError) Error() string {
	return fmt.Sprint(e.msg, " resource not implemented")
}

// Will hold parameters for each resource API
type InputParam struct {
	name  string
	value interface{}
}

// https://docs.aws.amazon.com/general/latest/gr/aws-arns-and-namespaces.html#arns-syntax
// arn:partition:service:region:account-id:resource-id
var resourceId = func(id string) string {
	return id
}

/*
// https://docs.aws.amazon.com/general/latest/gr/aws-arns-and-namespaces.html#arns-syntax
// arn:partition:service:region:account-id:resource-id
arnF1 := func(service string) func(string) string {
	return func(id string) string {
		return fmt.Sprintf("arn:aws:%s:%s:%s:%s",
			service, config.Region, getAccount(ctx, config), id)
	}
}
*/

// https://docs.aws.amazon.com/general/latest/gr/aws-arns-and-namespaces.html#arns-syntax
// arn:partition:service:region:account-id:resource-type/resource-id
var arnF2 = func(region string, account string, service string, resource string) func(string) string {
	return func(id string) string {
		return fmt.Sprintf("arn:aws:%s:%s:%s:%s/%s", service, region, account, resource, id)
	}
}

// arn:partition:service:region:account-id:resource-type:resource-id
var arnF3 = func(region string, account string, service string, resource string) func(string) string {
	return func(id string) string {
		return fmt.Sprintf("arn:aws:%s:%s:%s:%s:%s", service, region, account, resource, id)
	}
}

// For resources which don't support tagging
func nop(resourceType string) func(context.Context, aws.Config, string) (map[string]string, error) {
	return func(context.Context, aws.Config, string) (map[string]string, error) {
		return nil, &TagsNotSupportedError{resourceType}
	}
}

// Will use the tagLook parameter to call each resource API to get the tagging details
// Parameters for tagLook function will be received as an array of InputParam
func wrap(tagLookup interface{}, parameters...InputParam) func(ctx context.Context, config aws.Config, id string) (map[string]string, error) {
	t, fn := reflect.TypeOf(tagLookup), reflect.ValueOf(tagLookup)
	if t.Kind() != reflect.Func {
		panic(fmt.Errorf("wrap called on non-func type, %v", t))
	}
	if t.NumIn() != 1 {
		panic(fmt.Errorf("wrap func requires a single input parameter: got %v", t))
	}
	if t.NumOut() != 1 {
		panic(fmt.Errorf("wrap func requires a single output parameter: got %v", t))
	}
	inputType := t.In(0)
	for inputType.Kind() == reflect.Ptr {
		inputType = inputType.Elem()
	}
	return func(ctx context.Context, config aws.Config, id string) (map[string]string, error) {
		// create the Input object for each function call
		// by convention AWS uses an Input type for each operation
		input := reflect.New(inputType)
		arg := input.Interface()
		argType, argValue := reflect.TypeOf(arg), reflect.ValueOf(arg)
		for argType.Kind() == reflect.Ptr {
			argType = argType.Elem()
			argValue = argValue.Elem()
		}

		// fill in the required parameters to call AWS API
		// todo handle error checking
		for i := 0; i < argType.NumField(); i++ {
			field := argType.Field(i)
			for _, parameter := range parameters {
				if field.Name == parameter.name && argValue.Field(i).CanSet() {
					if reflect.Func == reflect.TypeOf(parameter.value).Kind() {
						value := (parameter.value.(func(string)string))(id)
						argValue.Field(i).Set(reflect.ValueOf(aws.String(value)))
					} else {
						argValue.Field(i).Set(reflect.ValueOf(parameter.value))
					}
				}
			}
		}

		// each Request struct has a Send method to perform the call to AWS API
		req := fn.Call([]reflect.Value{input})
		out := req[0].MethodByName("Send").Call([]reflect.Value{reflect.ValueOf(ctx)})
		if err, ok := out[1].Interface().(error); ok && err != nil {
			return nil, err
		}

		// store the response into outType/outValue and de-ref as necessary
		outType, outValue := reflect.TypeOf(out[0].Interface()), out[0]
		for outType.Kind() == reflect.Ptr {
			outType = outType.Elem()
			outValue = outValue.Elem()
		}
		// as a convention, aws return types are always wrapped types
		// that consist of the part we're interested (always the first
		// field) and the aws response (the second field)
		//
		// here we rebind the contents of the first field into
		// outType/outValue
		outType, outValue = outType.Field(0).Type, outValue.Field(0)
		for outType.Kind() == reflect.Ptr {
			outType = outType.Elem()
			outValue = outValue.Elem()
		}

		for i := 0; i < outType.NumField(); i++ {
			field := outType.Field(i)

			if containsString([]string{"Tags","TagSet","TagList"}, field.Name) {
				// some API's return a map
				tagsMap, ok := outValue.FieldByName(field.Name).Interface().(map[string]string)
				if ok {
					return tagsMap, nil
				}
				// some API's return an array of objects with Key & Value fields
				tags := map[string]string{}
				for _, tagsField := range []string{"Tags","TagSet","TagList"} {
					fieldValue := outValue.FieldByName(tagsField)
					if tagsField == field.Name && fieldValue.Kind() == reflect.Slice {
						for i := 0; i < fieldValue.Len(); i++ {
							item := fieldValue.Index(i)
							key := item.FieldByName("Key").Elem().String()
							value := item.FieldByName("Value").Elem().String()
							tags[key] = value
						}
					}
				}

				if len(tags) > 0 {
					return tags, nil
				} else {
					return nil, fmt.Errorf("unable to cast %s.%s: %s",
						outType.Name(), field.Name, Prettify(outValue.Interface()))
				}
			}
		}

		// no tags retrieved we don't know how to retrieve the tag details
		// or this response may not even the tags information
		return nil, fmt.Errorf("tags field not found in %s: %s",
			outType.Name(), Prettify(outValue.Interface()))
	}
}

func containsString(col []string, want string) bool {
	for _, s := range col {
		if s == want {
			return true
		}
	}
	return false
}
