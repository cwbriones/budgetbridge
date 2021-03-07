package splitwise

import (
	"fmt"
	"net/url"
	"strconv"
)

type valueWriter struct {
	url.Values
}

type requestWriter interface {
	Str(key, val string)
	Int(key string, val int)
}

func newRequest() valueWriter {
	return valueWriter{
		make(url.Values),
	}
}

func (r valueWriter) Str(key, val string) {
	r.Values.Set(key, val)
}

func (r valueWriter) Bool(key string, val bool) {
	r.Values.Set(key, strconv.FormatBool(val))
}

func (r valueWriter) Int(key string, val int) {
	r.Values.Set(key, strconv.Itoa(val))
}

func (r valueWriter) Array(key string) *arrayWriter {
	return &arrayWriter{
		rw:     r,
		prefix: key,
	}
}

type arrayWriter struct {
	rw     valueWriter
	prefix string
	i      int
}

func (a *arrayWriter) Next() {
	a.i++
}

func (a *arrayWriter) Str(key string, val string) {
	key = fmt.Sprintf("%s__%d__%s", a.prefix, a.i, key)
	a.rw.Str(key, val)
}

func (a *arrayWriter) Int(key string, val int) {
	key = fmt.Sprintf("%s__%d__%s", a.prefix, a.i, key)
	a.rw.Int(key, val)
}
