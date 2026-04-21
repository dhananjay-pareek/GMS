package webrunner

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"reflect"
	"sync"

	"github.com/gosom/scrapemate"
)

type csvJobWriter struct {
	file *os.File
	w    *csv.Writer
	once sync.Once
}

func newCSVJobWriter(filePath string) (scrapemate.ResultWriter, error) {
	file, err := os.Create(filePath)
	if err != nil {
		return nil, fmt.Errorf("create csv file: %w", err)
	}

	return &csvJobWriter{file: file, w: csv.NewWriter(file)}, nil
}

func (c *csvJobWriter) Run(_ context.Context, in <-chan scrapemate.Result) error {
	defer c.file.Close()
	defer c.w.Flush()

	for result := range in {
		elements, err := c.getCsvCapable(result.Data)
		if err != nil {
			return err
		}

		if len(elements) == 0 {
			continue
		}

		c.once.Do(func() {
			_ = c.w.Write(elements[0].CsvHeaders())
		})

		for _, element := range elements {
			if err := c.w.Write(element.CsvRow()); err != nil {
				return err
			}
		}
	}

	return c.w.Error()
}

func (c *csvJobWriter) getCsvCapable(data any) ([]scrapemate.CsvCapable, error) {
	var elements []scrapemate.CsvCapable

	switch v := data.(type) {
	case nil:
		return nil, nil
	case []any:
		for _, item := range v {
			nested, err := c.getCsvCapable(item)
			if err != nil {
				return nil, err
			}
			elements = append(elements, nested...)
		}
		return elements, nil
	}

	if interfaceIsSlice(data) {
		s := reflect.ValueOf(data)
		for i := 0; i < s.Len(); i++ {
			val := s.Index(i).Interface()
			if element, ok := val.(scrapemate.CsvCapable); ok {
				elements = append(elements, element)
			} else {
				return nil, fmt.Errorf("%w: unexpected data type: %T", scrapemate.ErrorNotCsvCapable, val)
			}
		}
		return elements, nil
	}

	if element, ok := data.(scrapemate.CsvCapable); ok {
		elements = append(elements, element)
		return elements, nil
	}

	return nil, fmt.Errorf("%w: unexpected data type: %T", scrapemate.ErrorNotCsvCapable, data)
}

func interfaceIsSlice(t any) bool {
	if t == nil {
		return false
	}

	//nolint:exhaustive // we only need to check for slices
	switch reflect.TypeOf(t).Kind() {
	case reflect.Slice:
		return true
	default:
		return false
	}
}
