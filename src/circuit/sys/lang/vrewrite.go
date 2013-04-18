// Copyright 2013 Tumblr, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package lang

import (
	"reflect"
	"sync"
)

// rewriteFunc can selectively rewrite a src value by writing
// the new value into dst and returning true.
// rewriteFunc MUST rewrite map values.
type rewriteFunc func(src, dst reflect.Value) bool

func rewriteInterface(rewrite rewriteFunc, src interface{}) interface{} {
	v := reflect.ValueOf(src)
	if isTerminalKind(v.Type()) {
		return src
	}
	pz := reflect.New(v.Type())
	if rewriteValue(rewrite, v, pz.Elem()) {
		return pz.Elem().Interface()
	}
	return src
}

type rewriteGroup struct {
	sync.WaitGroup
	sync.Mutex
	Change bool
}

// dst must be addressable.
func rewriteValue(rewrite rewriteFunc, src, dst reflect.Value) bool {
	if rewrite(src, dst) {
		return true
	}

	// Recursive step
	var g rewriteGroup
	t := src.Type()
	switch src.Kind() {

	case reflect.Array:
		if isTerminalKind(t) {
			return false
		}
		for i := 0; i < src.Len(); i++ {
			src__, dst__ := src.Index(i), dst.Index(i)
			g.Add(1)
			go func() {
				defer g.Done()
				if rewriteValue(rewrite, src__, dst__) {
					g.Lock()
					g.Change = true
					g.Unlock()
				} else {
					dst__.Set(src__)
				}
			}()
		}
		g.Wait()
		return g.Change

	case reflect.Slice:
		if src.IsNil() || isTerminalKind(t) {
			return false
		}
		dst.Set(reflect.MakeSlice(t, src.Len(), src.Len()))
		for i := 0; i < src.Len(); i++ {
			src__, dst__ := src.Index(i), dst.Index(i)
			g.Add(1)
			go func() {
				defer g.Done()
				if rewriteValue(rewrite, src__, dst__) {
					g.Lock()
					g.Change = true
					g.Unlock()
				} else {
					dst__.Set(src__)
				}
			}()
		}
		g.Wait()
		return g.Change

	case reflect.Map:
		// For now, we do not rewrite map key values
		if src.IsNil() || isTerminalKind(t) {
			return false
		}
		dst.Set(reflect.MakeMap(t))
		for _, mk := range src.MapKeys() {
			src__ := src.MapIndex(mk)
			dst__ := reflect.New(t.Elem()).Elem()
			g.Add(1)
			go func() {
				defer g.Done()
				if rewriteValue(rewrite, src__, dst__) {
					dst.SetMapIndex(mk, dst__)
					g.Lock()
					g.Change = true
					g.Unlock()
				} else {
					dst.SetMapIndex(mk, src__)
				}
			}()
		}
		g.Wait()
		return g.Change

	case reflect.Ptr:
		if src.IsNil() || isTerminalKind(t) {
			return false
		}
		pz := reflect.New(t.Elem())
		if rewriteValue(rewrite, src.Elem(), pz.Elem()) {
			dst.Set(pz)
			return true
		}
		return false

	case reflect.Interface:
		if !src.IsNil() {
			if src.Elem().Kind() == reflect.Ptr && src.Elem().IsNil() {
				// Rewrite src to be a <nil,nil> interface value, instead of <*int,nil>
				dst.Set(reflect.Zero(t))
				return true
			}
		}
		// If value is nil of type *T, collapse to absolute nil
		if src.IsNil() || isTerminalKind(src.Elem().Type()) {
			return false
		}

		// Recursive interface value unflattening would happen here;
		// We don't use it, however, since there is no source of
		// type-information for the actual passed values.
		pz := reflect.New(src.Elem().Type())
		if rewriteValue(rewrite, src.Elem(), pz.Elem()) {
			dst.Set(pz.Elem())
			return true
		}
		return false

	case reflect.Struct:
		if isTerminalKind(t) {
			return false
		}
		for i := 0; i < src.NumField(); i++ {
			if t.Field(i).PkgPath == "" {
				// If field is public
				src__, dst__ := src.Field(i), dst.Field(i)
				g.Add(1)
				go func() {
					defer g.Done()
					if rewriteValue(rewrite, src__, dst__) {
						g.Lock()
						g.Change = true
						g.Unlock()
					} else {
						dst__.Set(src__)
					}
				}()
			}
		}
		g.Wait()
		return g.Change

	case reflect.Chan:
		panic("rewrite chan, not supported yet")

	case reflect.Func:
		panic("rewrite func")

	case reflect.UnsafePointer:
		panic("rewrite unsafe pointer")
	}

	// All remaining types are primitive and therefore terminal
	return false
}

func isTerminalKind(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Array, reflect.Slice, reflect.Ptr:
		return isTerminalKind(t.Elem())
	case reflect.Map:
		// For now, we do not rewrite map key values
		return isTerminalKind(t.Elem())
	case reflect.Interface:
		return false
	case reflect.Struct:
		terminal := true
		for i := 0; i < t.NumField(); i++ {
			if t.Field(i).PkgPath == "" {
				// If field is public
				if !isTerminalKind(t.Field(i).Type) {
					terminal = false
					break
				}
			}
		}
		return terminal
	}
	return true
}
