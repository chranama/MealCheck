package evalexport

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
)

// WriteJSONL writes one JSON object per row. Rows must be a slice.
func WriteJSONL(path string, rows any) error {
	values, _, err := sliceValues(rows)
	if err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	for i := 0; i < values.Len(); i++ {
		if err := encoder.Encode(values.Index(i).Interface()); err != nil {
			return fmt.Errorf("encode row %d: %w", i+1, err)
		}
	}
	return nil
}

// WriteCSV writes flat struct rows with headers derived from json tags.
func WriteCSV(path string, rows any) error {
	values, fields, err := sliceValues(rows)
	if err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	writer := csv.NewWriter(file)

	header := make([]string, 0, len(fields))
	for _, field := range fields {
		header = append(header, field.name)
	}
	if err := writer.Write(header); err != nil {
		return err
	}
	for i := 0; i < values.Len(); i++ {
		value := values.Index(i)
		if value.Kind() == reflect.Pointer {
			value = value.Elem()
		}
		row := make([]string, 0, len(fields))
		for _, field := range fields {
			row = append(row, csvFieldValue(value.Field(field.index)))
		}
		if err := writer.Write(row); err != nil {
			return fmt.Errorf("write row %d: %w", i+1, err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return err
	}
	return nil
}

type csvField struct {
	index int
	name  string
}

func sliceValues(rows any) (reflect.Value, []csvField, error) {
	value := reflect.ValueOf(rows)
	if value.Kind() != reflect.Slice {
		return reflect.Value{}, nil, fmt.Errorf("export rows must be a slice")
	}
	elementType := value.Type().Elem()
	if elementType.Kind() == reflect.Pointer {
		elementType = elementType.Elem()
	}
	if elementType.Kind() != reflect.Struct {
		return reflect.Value{}, nil, fmt.Errorf("export rows must contain structs")
	}
	fields := make([]csvField, 0, elementType.NumField())
	for i := 0; i < elementType.NumField(); i++ {
		field := elementType.Field(i)
		if !field.IsExported() {
			continue
		}
		name := csvHeaderName(field)
		if name == "" {
			continue
		}
		fields = append(fields, csvField{index: i, name: name})
	}
	return value, fields, nil
}

func csvHeaderName(field reflect.StructField) string {
	tag := field.Tag.Get("json")
	if tag == "-" {
		return ""
	}
	name, _, _ := strings.Cut(tag, ",")
	if name != "" {
		return name
	}
	return field.Name
}

func csvFieldValue(value reflect.Value) string {
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return ""
		}
		value = value.Elem()
	}
	switch value.Kind() {
	case reflect.String:
		return value.String()
	case reflect.Bool:
		return strconv.FormatBool(value.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(value.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return strconv.FormatUint(value.Uint(), 10)
	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(value.Float(), 'f', -1, value.Type().Bits())
	case reflect.Slice, reflect.Array:
		return csvSliceValue(value)
	default:
		return fmt.Sprint(value.Interface())
	}
}

func csvSliceValue(value reflect.Value) string {
	parts := make([]string, 0, value.Len())
	for i := 0; i < value.Len(); i++ {
		parts = append(parts, csvFieldValue(value.Index(i)))
	}
	return strings.Join(parts, "|")
}
