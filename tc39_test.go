package goja

import (
	"errors"
	"fmt"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"sync"
	"testing"
)

const (
	tc39BASE = "testdata/test262"
)

var (
	invalidFormatError = errors.New("Invalid file format")

	ignorableTestError = &valueSymbol{}

	sabStub = MustCompile("sabStub.js", `
		Object.defineProperty(this, "SharedArrayBuffer", {
			get: function() {
				throw IgnorableTestError;
			}
		});`,
		false)
)

var (
	skipList = map[string]bool{
		"test/language/literals/regexp/S7.8.5_A1.1_T2.js":             true, // UTF-16
		"test/language/literals/regexp/S7.8.5_A1.4_T2.js":             true, // UTF-16
		"test/language/literals/regexp/S7.8.5_A2.1_T2.js":             true, // UTF-16
		"test/language/literals/regexp/S7.8.5_A2.4_T2.js":             true, // UTF-16
		"test/built-ins/Date/prototype/toISOString/15.9.5.43-0-8.js":  true, // timezone
		"test/built-ins/Date/prototype/toISOString/15.9.5.43-0-9.js":  true, // timezone
		"test/built-ins/Date/prototype/toISOString/15.9.5.43-0-10.js": true, // timezone
		"test/annexB/built-ins/escape/escape-above-astral.js":         true, // \u{xxxxx}

		// utf-16
		"test/built-ins/Array/prototype/concat/Array.prototype.concat_spreadable-string-wrapper.js": true,

		// class
		"test/language/statements/class/subclass/builtin-objects/Symbol/symbol-valid-as-extends-value.js": true,
		"test/language/statements/class/subclass/builtin-objects/Symbol/new-symbol-with-super-throws.js":  true,
		"test/language/statements/class/subclass/builtin-objects/WeakSet/super-must-be-called.js":         true,
		"test/language/statements/class/subclass/builtin-objects/WeakSet/regular-subclassing.js":          true,
		"test/language/statements/class/subclass/builtin-objects/WeakMap/super-must-be-called.js":         true,
		"test/language/statements/class/subclass/builtin-objects/WeakMap/regular-subclassing.js":          true,
		"test/language/statements/class/subclass/builtin-objects/Map/super-must-be-called.js":             true,
		"test/language/statements/class/subclass/builtin-objects/Map/regular-subclassing.js":              true,
		"test/language/statements/class/subclass/builtin-objects/Set/super-must-be-called.js":             true,
		"test/language/statements/class/subclass/builtin-objects/Set/regular-subclassing.js":              true,
		"test/language/statements/class/subclass/builtin-objects/Object/replacing-prototype.js":           true,
		"test/language/statements/class/subclass/builtin-objects/Object/regular-subclassing.js":           true,
		"test/built-ins/Array/prototype/concat/Array.prototype.concat_non-array.js":                       true,
		"test/language/statements/class/subclass/builtin-objects/Array/length.js":                         true,
		"test/language/statements/class/subclass/builtin-objects/TypedArray/super-must-be-called.js":      true,
		"test/language/statements/class/subclass/builtin-objects/TypedArray/regular-subclassing.js":       true,
		"test/language/statements/class/subclass/builtin-objects/DataView/super-must-be-called.js":        true,
		"test/language/statements/class/subclass/builtin-objects/DataView/regular-subclassing.js":         true,

		// full unicode regexp flag
		"test/built-ins/RegExp/prototype/Symbol.match/u-advance-after-empty.js":               true,
		"test/built-ins/RegExp/prototype/Symbol.match/get-unicode-error.js":                   true,
		"test/built-ins/RegExp/prototype/Symbol.match/builtin-success-u-return-val-groups.js": true,
		"test/built-ins/RegExp/prototype/Symbol.match/builtin-infer-unicode.js":               true,

		// object literals
		"test/built-ins/Array/from/source-object-iterator-1.js":                   true,
		"test/built-ins/Array/from/source-object-iterator-2.js":                   true,
		"test/built-ins/TypedArray/prototype/fill/fill-values-conversion-once.js": true,
		"test/built-ins/TypedArrays/of/this-is-not-constructor.js":                true,
		"test/built-ins/TypedArrays/of/argument-number-value-throws.js":           true,
		"test/built-ins/TypedArrays/from/this-is-not-constructor.js":              true,
		"test/built-ins/TypedArrays/from/set-value-abrupt-completion.js":          true,
		"test/built-ins/TypedArrays/from/property-abrupt-completion.js":           true,
		"test/built-ins/TypedArray/of/this-is-not-constructor.js":                 true,
		"test/built-ins/TypedArray/from/this-is-not-constructor.js":               true,
		"test/built-ins/DataView/custom-proto-access-throws.js":                   true,
		"test/built-ins/DataView/custom-proto-access-throws-sab.js":               true,

		// arrow-function
		"test/built-ins/Object/prototype/toString/proxy-function.js": true,
	}

	featuresBlackList = []string{
		"arrow-function",
	}

	es6WhiteList = map[string]bool{}

	es6IdWhiteList = []string{
		"8.1.2.1",
		"9.5",
		"12.9.3",
		"12.9.4",
		"19.1",
		"19.4",
		"21.1.3.14",
		"21.1.3.15",
		"21.1.3.17",
		"21.2.5.6",
		"22.1.2.1",
		"22.1.2.3",
		"22.1.2.5",
		"22.1.3",
		"22.1.4",
		"22.2",
		"23.1",
		"23.2",
		"23.3",
		"23.4",
		"24.2",
		"25.1.2",
		"26.1",
		"26.2",
		"B.2.1",
		"B.2.2",
	}

	esIdPrefixWhiteList = []string{
		"sec-array.prototype.includes",
		"sec-%typedarray%",
	}
)

type tc39Test struct {
	name string
	f    func(t *testing.T)
}

type tc39TestCtx struct {
	base         string
	t            *testing.T
	prgCache     map[string]*Program
	prgCacheLock sync.Mutex
	testQueue    []tc39Test
}

type TC39MetaNegative struct {
	Phase, Type string
}

type tc39Meta struct {
	Negative TC39MetaNegative
	Includes []string
	Flags    []string
	Features []string
	Es5id    string
	Es6id    string
	Esid     string
}

func (m *tc39Meta) hasFlag(flag string) bool {
	for _, f := range m.Flags {
		if f == flag {
			return true
		}
	}
	return false
}

func parseTC39File(name string) (*tc39Meta, string, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()

	b, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, "", err
	}

	str := string(b)
	metaStart := strings.Index(str, "/*---")
	if metaStart == -1 {
		return nil, "", invalidFormatError
	} else {
		metaStart += 5
	}
	metaEnd := strings.Index(str, "---*/")
	if metaEnd == -1 || metaEnd <= metaStart {
		return nil, "", invalidFormatError
	}

	var meta tc39Meta
	err = yaml.Unmarshal([]byte(str[metaStart:metaEnd]), &meta)
	if err != nil {
		return nil, "", err
	}

	if meta.Negative.Type != "" && meta.Negative.Phase == "" {
		return nil, "", errors.New("negative type is set, but phase isn't")
	}

	return &meta, str, nil
}

func (*tc39TestCtx) detachArrayBuffer(call FunctionCall) Value {
	if obj, ok := call.Argument(0).(*Object); ok {
		if buf, ok := obj.self.(*arrayBufferObject); ok {
			buf.detach()
			return _undefined
		}
	}
	panic(typeError("detachArrayBuffer() is called with incompatible argument"))
}

func (*tc39TestCtx) throwIgnorableTestError(FunctionCall) Value {
	panic(ignorableTestError)
}

func (ctx *tc39TestCtx) runTC39Test(name, src string, meta *tc39Meta, t testing.TB) {
	defer func() {
		if x := recover(); x != nil {
			panic(fmt.Sprintf("panic while running %s: %v", name, x))
		}
	}()
	vm := New()
	_262 := vm.NewObject()
	_262.Set("detachArrayBuffer", ctx.detachArrayBuffer)
	_262.Set("createRealm", ctx.throwIgnorableTestError)
	vm.Set("$262", _262)
	vm.Set("IgnorableTestError", ignorableTestError)
	vm.RunProgram(sabStub)
	err, early := ctx.runTC39Script(name, src, meta.Includes, vm)

	if err != nil {
		if meta.Negative.Type == "" {
			if err, ok := err.(*Exception); ok {
				if err.Value() == ignorableTestError {
					t.Skip("Test threw IgnorableTestError")
				}
			}
			t.Fatalf("%s: %v", name, err)
		} else {
			if meta.Negative.Phase == "early" && !early || meta.Negative.Phase == "runtime" && early {
				t.Fatalf("%s: error %v happened at the wrong phase (expected %s)", name, err, meta.Negative.Phase)
			}
			var errType string

			switch err := err.(type) {
			case *Exception:
				if o, ok := err.Value().(*Object); ok {
					if c := o.Get("constructor"); c != nil {
						if c, ok := c.(*Object); ok {
							errType = c.Get("name").String()
						} else {
							t.Fatalf("%s: error constructor is not an object (%v)", name, o)
						}
					} else {
						t.Fatalf("%s: error does not have a constructor (%v)", name, o)
					}
				} else {
					t.Fatalf("%s: error is not an object (%v)", name, err.Value())
				}
			case *CompilerSyntaxError:
				errType = "SyntaxError"
			case *CompilerReferenceError:
				errType = "ReferenceError"
			default:
				t.Fatalf("%s: error is not a JS error: %v", name, err)
			}

			if errType != meta.Negative.Type {
				vm.vm.prg.dumpCode(t.Logf)
				t.Fatalf("%s: unexpected error type (%s), expected (%s)", name, errType, meta.Negative.Type)
			}
		}
	} else {
		if meta.Negative.Type != "" {
			vm.vm.prg.dumpCode(t.Logf)
			t.Fatalf("%s: Expected error: %v", name, err)
		}
	}
}

func (ctx *tc39TestCtx) runTC39File(name string, t testing.TB) {
	if skipList[name] {
		t.Skip("Excluded")
	}
	p := path.Join(ctx.base, name)
	meta, src, err := parseTC39File(p)
	if err != nil {
		//t.Fatalf("Could not parse %s: %v", name, err)
		t.Errorf("Could not parse %s: %v", name, err)
		return
	}
	if meta.Es5id == "" {
		skip := true
		//t.Logf("%s: Not ES5, skipped", name)
		if es6WhiteList[name] {
			skip = false
		} else {
			if meta.Es6id != "" {
				for _, prefix := range es6IdWhiteList {
					if strings.HasPrefix(meta.Es6id, prefix) &&
						(len(meta.Es6id) == len(prefix) || meta.Es6id[len(prefix)] == '.') {

						skip = false
						break
					}
				}
			}
		}
		if skip {
			if meta.Esid != "" {
				for _, prefix := range esIdPrefixWhiteList {
					if strings.HasPrefix(meta.Esid, prefix) &&
						(len(meta.Esid) == len(prefix) || meta.Esid[len(prefix)] == '.') {

						skip = false
						break
					}
				}
			}
		}
		if skip {
			t.Skip("Not ES5")
		}

		for _, feature := range meta.Features {
			for _, bl := range featuresBlackList {
				if feature == bl {
					t.Skip("Blacklisted feature")
				}
			}
		}
	}

	hasRaw := meta.hasFlag("raw")

	if hasRaw || !meta.hasFlag("onlyStrict") {
		//log.Printf("Running normal test: %s", name)
		//t.Logf("Running normal test: %s", name)
		ctx.runTC39Test(name, src, meta, t)
	}

	if !hasRaw && !meta.hasFlag("noStrict") {
		//log.Printf("Running strict test: %s", name)
		//t.Logf("Running strict test: %s", name)
		ctx.runTC39Test(name, "'use strict';\n"+src, meta, t)
	}

}

func (ctx *tc39TestCtx) init() {
	ctx.prgCache = make(map[string]*Program)
}

func (ctx *tc39TestCtx) compile(base, name string) (*Program, error) {
	ctx.prgCacheLock.Lock()
	defer ctx.prgCacheLock.Unlock()

	prg := ctx.prgCache[name]
	if prg == nil {
		fname := path.Join(base, name)
		f, err := os.Open(fname)
		if err != nil {
			return nil, err
		}
		defer f.Close()

		b, err := ioutil.ReadAll(f)
		if err != nil {
			return nil, err
		}

		str := string(b)
		prg, err = Compile(name, str, false)
		if err != nil {
			return nil, err
		}
		ctx.prgCache[name] = prg
	}

	return prg, nil
}

func (ctx *tc39TestCtx) runFile(base, name string, vm *Runtime) error {
	prg, err := ctx.compile(base, name)
	if err != nil {
		return err
	}
	_, err = vm.RunProgram(prg)
	return err
}

func (ctx *tc39TestCtx) runTC39Script(name, src string, includes []string, vm *Runtime) (err error, early bool) {
	early = true
	err = ctx.runFile(ctx.base, path.Join("harness", "assert.js"), vm)
	if err != nil {
		return
	}

	err = ctx.runFile(ctx.base, path.Join("harness", "sta.js"), vm)
	if err != nil {
		return
	}

	for _, include := range includes {
		err = ctx.runFile(ctx.base, path.Join("harness", include), vm)
		if err != nil {
			return
		}
	}

	var p *Program
	p, err = Compile(name, src, false)

	if err != nil {
		return
	}

	early = false
	_, err = vm.RunProgram(p)

	return
}

func (ctx *tc39TestCtx) runTC39Tests(name string) {
	files, err := ioutil.ReadDir(path.Join(ctx.base, name))
	if err != nil {
		ctx.t.Fatal(err)
	}

	for _, file := range files {
		if file.Name()[0] == '.' {
			continue
		}
		if file.IsDir() {
			ctx.runTC39Tests(path.Join(name, file.Name()))
		} else {
			if strings.HasSuffix(file.Name(), ".js") {
				name := path.Join(name, file.Name())
				ctx.runTest(name, func(t *testing.T) {
					ctx.runTC39File(name, t)
				})
			}
		}
	}

}

func TestTC39(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	if _, err := os.Stat(tc39BASE); err != nil {
		t.Skipf("If you want to run tc39 tests, download them from https://github.com/tc39/test262 and put into %s. The last working commit is 1ba3a7c4a93fc93b3d0d7e4146f59934a896837d. (%v)", tc39BASE, err)
	}

	ctx := &tc39TestCtx{
		base: tc39BASE,
		t:    t,
	}
	ctx.init()

	//ctx.runTC39File("test/language/types/number/8.5.1.js", t)
	//ctx.runTC39Tests("test/language")
	ctx.runTC39Tests("test/language/expressions")
	ctx.runTC39Tests("test/language/arguments-object")
	ctx.runTC39Tests("test/language/asi")
	ctx.runTC39Tests("test/language/directive-prologue")
	ctx.runTC39Tests("test/language/function-code")
	ctx.runTC39Tests("test/language/eval-code")
	ctx.runTC39Tests("test/language/global-code")
	ctx.runTC39Tests("test/language/identifier-resolution")
	ctx.runTC39Tests("test/language/identifiers")
	//ctx.runTC39Tests("test/language/literals") // octal sequences in strict mode
	ctx.runTC39Tests("test/language/punctuators")
	ctx.runTC39Tests("test/language/reserved-words")
	ctx.runTC39Tests("test/language/source-text")
	ctx.runTC39Tests("test/language/statements")
	ctx.runTC39Tests("test/language/types")
	ctx.runTC39Tests("test/language/white-space")
	ctx.runTC39Tests("test/built-ins")
	ctx.runTC39Tests("test/annexB/built-ins/String/prototype/substr")
	ctx.runTC39Tests("test/annexB/built-ins/escape")
	ctx.runTC39Tests("test/annexB/built-ins/unescape")

	ctx.flush()
}
