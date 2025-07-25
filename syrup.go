package main

import (
	"fmt"
	"go/types"
	"io"
	"sort"
	"strings"
	"text/template"

	"github.com/ettle/strcase"
)

const (
	templateImports = `// Code generated by mocktail; DO NOT EDIT.

package {{ .Name }}

{{ if .Imports }}import (
{{- range $index, $import := .Imports }}
	{{ if $import }}"{{ $import }}"{{ else }}{{end}}
{{- end}}
){{end}}
`

	templateMockBase = `
// {{ .InterfaceName | ToGoCamel }}Mock mock of {{ .InterfaceName }}.
type {{ .InterfaceName | ToGoCamel }}Mock{{ .TypeParamsDecl }} struct { mock.Mock }

// {{.ConstructorPrefix}}{{ .InterfaceName | ToGoPascal }}Mock creates a new {{ .InterfaceName | ToGoCamel }}Mock.
func {{.ConstructorPrefix}}{{ .InterfaceName | ToGoPascal }}Mock{{ .TypeParamsDecl }}(tb testing.TB) *{{ .InterfaceName | ToGoCamel }}Mock{{ .TypeParamsUse }} {
	tb.Helper()

	m := &{{ .InterfaceName | ToGoCamel }}Mock{{ .TypeParamsUse }}{}
	m.Mock.Test(tb)

	tb.Cleanup(func() { m.AssertExpectations(tb) })

	return m
}
`

	templateCallBase = `
type {{ .InterfaceName | ToGoCamel }}{{ .MethodName }}Call{{ .TypeParamsDecl }} struct{
	*mock.Call
	Parent *{{ .InterfaceName | ToGoCamel }}Mock{{ .TypeParamsUse }}
}


func (_c *{{ .InterfaceName | ToGoCamel }}{{ .MethodName }}Call{{ .TypeParamsUse }}) Panic(msg string) *{{ .InterfaceName | ToGoCamel }}{{ .MethodName }}Call{{ .TypeParamsUse }} {
	_c.Call = _c.Call.Panic(msg)
	return _c
}

func (_c *{{ .InterfaceName | ToGoCamel }}{{ .MethodName }}Call{{ .TypeParamsUse }}) Once() *{{ .InterfaceName | ToGoCamel }}{{ .MethodName }}Call{{ .TypeParamsUse }} {
	_c.Call = _c.Call.Once()
	return _c
}

func (_c *{{ .InterfaceName | ToGoCamel }}{{ .MethodName }}Call{{ .TypeParamsUse }}) Twice() *{{ .InterfaceName | ToGoCamel }}{{ .MethodName }}Call{{ .TypeParamsUse }} {
	_c.Call = _c.Call.Twice()
	return _c
}

func (_c *{{ .InterfaceName | ToGoCamel }}{{ .MethodName }}Call{{ .TypeParamsUse }}) Times(i int) *{{ .InterfaceName | ToGoCamel }}{{ .MethodName }}Call{{ .TypeParamsUse }} {
	_c.Call = _c.Call.Times(i)
	return _c
}

func (_c *{{ .InterfaceName | ToGoCamel }}{{ .MethodName }}Call{{ .TypeParamsUse }}) WaitUntil(w <-chan time.Time) *{{ .InterfaceName | ToGoCamel }}{{ .MethodName }}Call{{ .TypeParamsUse }} {
	_c.Call = _c.Call.WaitUntil(w)
	return _c
}

func (_c *{{ .InterfaceName | ToGoCamel }}{{ .MethodName }}Call{{ .TypeParamsUse }}) After(d time.Duration) *{{ .InterfaceName | ToGoCamel }}{{ .MethodName }}Call{{ .TypeParamsUse }} {
	_c.Call = _c.Call.After(d)
	return _c
}

func (_c *{{ .InterfaceName | ToGoCamel }}{{ .MethodName }}Call{{ .TypeParamsUse }}) Run(fn func(args mock.Arguments)) *{{ .InterfaceName | ToGoCamel }}{{ .MethodName }}Call{{ .TypeParamsUse }} {
	_c.Call = _c.Call.Run(fn)
	return _c
}

func (_c *{{ .InterfaceName | ToGoCamel }}{{ .MethodName }}Call{{ .TypeParamsUse }}) Maybe() *{{ .InterfaceName | ToGoCamel }}{{ .MethodName }}Call{{ .TypeParamsUse }} {
	_c.Call = _c.Call.Maybe()
	return _c
}

`
)

// Syrup generates method mocks and mock.Call wrapper.
type Syrup struct {
	PkgPath       string
	InterfaceName string
	Method        *types.Func
	Signature     *types.Signature
	TypeParams    *types.TypeParamList
}

// Call generates mock.Call wrapper.
func (s Syrup) Call(writer io.Writer, methods []*types.Func) error {
	err := s.callBase(writer)
	if err != nil {
		return err
	}

	err = s.typedReturns(writer)
	if err != nil {
		return err
	}

	err = s.returnsFn(writer)
	if err != nil {
		return err
	}

	err = s.typedRun(writer)
	if err != nil {
		return err
	}

	err = s.callMethodsOn(writer, methods)
	if err != nil {
		return err
	}

	return s.callMethodOnRaw(writer, methods)
}

// MockMethod generates method mocks.
func (s Syrup) MockMethod(writer io.Writer) error {
	err := s.mockedMethod(writer)
	if err != nil {
		return err
	}

	err = s.methodOn(writer)
	if err != nil {
		return err
	}

	return s.methodOnRaw(writer)
}

// getTypeParamsUse returns type parameters for usage in method receivers.
func (s Syrup) getTypeParamsUse() string {
	if s.TypeParams == nil || s.TypeParams.Len() == 0 {
		return ""
	}

	var names []string
	for i := range s.TypeParams.Len() {
		tp := s.TypeParams.At(i)
		names = append(names, tp.Obj().Name())
	}
	return "[" + strings.Join(names, ", ") + "]"
}

func (s Syrup) mockedMethod(writer io.Writer) error {
	w := &Writer{writer: writer}

	typeParamsUse := s.getTypeParamsUse()
	w.Printf("func (_m *%sMock%s) %s(", strcase.ToGoCamel(s.InterfaceName), typeParamsUse, s.Method.Name())

	params := s.Signature.Params()

	var argNames []string
	for i := range params.Len() {
		param := params.At(i)

		if param.Type().String() == contextType {
			w.Print("_")
		} else {
			name := getParamName(param, i)
			w.Print(name)
			argNames = append(argNames, name)
		}

		w.Print(" " + s.getTypeName(param.Type(), i == params.Len()-1))

		if i+1 < params.Len() {
			w.Print(", ")
		}
	}

	w.Print(") ")

	results := s.Signature.Results()

	if results.Len() > 1 {
		w.Print("(")
	}

	for i := range results.Len() {
		w.Print(s.getTypeName(results.At(i).Type(), false))
		if i+1 < results.Len() {
			w.Print(", ")
		}
	}

	if results.Len() > 1 {
		w.Print(")")
	}

	w.Println(" {")

	w.Print("\t")
	if results.Len() > 0 {
		w.Print("_ret := ")
	}
	w.Printf("_m.Called(%s)\n", strings.Join(argNames, ", "))

	s.writeReturnsFnCaller(w, argNames, params, results)

	for i := range results.Len() {
		if i == 0 {
			w.Println()
		}

		rType := results.At(i).Type()

		w.Printf("\t%s", getResultName(results.At(i), i))

		switch rType.String() {
		case "string", "int", "bool", "error":
			w.Printf("\t := _ret.%s(%d)\n", strcase.ToPascal(rType.String()), i)
		default:
			name := s.getTypeName(rType, false)
			w.Printf(", _ := _ret.Get(%d).(%s)\n", i, name)
		}
	}

	for i := range results.Len() {
		if i == 0 {
			w.Println()
			w.Print("\treturn ")
		}

		w.Print(getResultName(results.At(i), i))

		if i+1 < results.Len() {
			w.Print(", ")
		} else {
			w.Println()
		}
	}

	w.Println("}")
	w.Println()

	return w.Err()
}

func (s Syrup) writeReturnsFnCaller(w *Writer, argNames []string, params, results *types.Tuple) {
	if results.Len() > 0 {
		w.Println()
		w.Printf("\tif _rf, ok := _ret.Get(0).(%s); ok {\n", s.createFuncSignature(params, results))
		w.Printf("\t\treturn _rf(%s", strings.Join(argNames, ", "))
		if s.Signature.Variadic() {
			w.Print("...")
		}
		w.Println(")")
		w.Println("\t}")
	}
}

func (s Syrup) methodOn(writer io.Writer) error {
	w := &Writer{writer: writer}

	structBaseName := strcase.ToGoCamel(s.InterfaceName)

	typeParamsUse := s.getTypeParamsUse()
	w.Printf("func (_m *%sMock%s) On%s(", structBaseName, typeParamsUse, s.Method.Name())

	params := s.Signature.Params()

	var argNames []string
	for i := range params.Len() {
		param := params.At(i)

		if param.Type().String() == contextType {
			continue
		}

		name := getParamName(param, i)

		w.Print(name)

		if _, ok := param.Type().(*types.Signature); ok {
			argNames = append(argNames, "mock.Anything")
		} else {
			argNames = append(argNames, name)
		}

		w.Print(" " + s.getTypeName(param.Type(), i == params.Len()-1))

		if i+1 < params.Len() {
			w.Print(", ")
		}
	}

	w.Printf(") *%s%sCall%s {\n", structBaseName, s.Method.Name(), typeParamsUse)

	w.Printf(`	return &%s%sCall%s{Call: _m.Mock.On("%s", %s), Parent: _m}`,
		structBaseName, s.Method.Name(), typeParamsUse, s.Method.Name(), strings.Join(argNames, ", "))

	w.Println()
	w.Println("}")
	w.Println()

	return w.Err()
}

func (s Syrup) methodOnRaw(writer io.Writer) error {
	w := &Writer{writer: writer}

	structBaseName := strcase.ToGoCamel(s.InterfaceName)

	typeParamsUse := s.getTypeParamsUse()
	w.Printf("func (_m *%sMock%s) On%sRaw(", structBaseName, typeParamsUse, s.Method.Name())

	params := s.Signature.Params()

	var argNames []string
	for i := range params.Len() {
		param := params.At(i)

		if param.Type().String() == contextType {
			continue
		}

		name := getParamName(param, i)

		w.Print(name)

		if _, ok := param.Type().(*types.Signature); ok {
			argNames = append(argNames, "mock.Anything")
		} else {
			argNames = append(argNames, name)
		}

		w.Print(" interface{}")

		if i+1 < params.Len() {
			w.Print(", ")
		}
	}

	w.Printf(") *%s%sCall%s {\n", structBaseName, s.Method.Name(), typeParamsUse)

	w.Printf(`	return &%s%sCall%s{Call: _m.Mock.On("%s", %s), Parent: _m}`,
		structBaseName, s.Method.Name(), typeParamsUse, s.Method.Name(), strings.Join(argNames, ", "))

	w.Println()
	w.Println("}")
	w.Println()

	return w.Err()
}

func (s Syrup) callBase(writer io.Writer) error {
	base := template.New("templateCallBase").Funcs(template.FuncMap{
		"ToGoCamel":  strcase.ToGoCamel,
		"ToGoPascal": strcase.ToGoPascal,
	})

	tmpl, err := base.Parse(templateCallBase)
	if err != nil {
		return err
	}

	// Generate type parameter declarations and usage
	typeParamsDecl := ""
	typeParamsUse := ""
	if s.TypeParams != nil && s.TypeParams.Len() > 0 {
		var params []string
		var names []string
		for i := range s.TypeParams.Len() {
			tp := s.TypeParams.At(i)
			params = append(params, tp.Obj().Name()+" "+tp.Constraint().String())
			names = append(names, tp.Obj().Name())
		}
		typeParamsDecl = "[" + strings.Join(params, ", ") + "]"
		typeParamsUse = "[" + strings.Join(names, ", ") + "]"
	}

	data := map[string]string{
		"InterfaceName":  s.InterfaceName,
		"MethodName":     s.Method.Name(),
		"TypeParamsDecl": typeParamsDecl,
		"TypeParamsUse":  typeParamsUse,
	}

	return tmpl.Execute(writer, data)
}

func (s Syrup) typedReturns(writer io.Writer) error {
	w := &Writer{writer: writer}

	results := s.Signature.Results()
	if results.Len() <= 0 {
		return nil
	}

	structBaseName := strcase.ToGoCamel(s.InterfaceName)

	typeParamsUse := s.getTypeParamsUse()
	w.Printf("func (_c *%s%sCall%s) TypedReturns(", structBaseName, s.Method.Name(), typeParamsUse)

	var returnNames string
	for i := range results.Len() {
		rName := string(rune(int('a') + i))

		w.Printf("%s %s", rName, s.getTypeName(results.At(i).Type(), false))
		returnNames += rName

		if i+1 < results.Len() {
			w.Print(", ")
			returnNames += ", "
		}
	}

	w.Printf(") *%s%sCall%s {\n", structBaseName, s.Method.Name(), typeParamsUse)
	w.Printf("\t_c.Call = _c.Return(%s)\n", returnNames)
	w.Println("\treturn _c")
	w.Println("}")
	w.Println()

	return w.Err()
}

func (s Syrup) typedRun(writer io.Writer) error {
	w := &Writer{writer: writer}

	params := s.Signature.Params()

	structBaseName := strcase.ToGoCamel(s.InterfaceName)

	w.Printf("func (_c *%[1]s%[2]sCall%[4]s) TypedRun(fn %[3]s) *%[1]s%[2]sCall%[4]s {\n",
		structBaseName, s.Method.Name(), s.createFuncSignature(params, nil), s.getTypeParamsUse())
	w.Println("\t_c.Call = _c.Call.Run(func(args mock.Arguments) {")

	var pos int
	var paramNames []string
	for i := range params.Len() {
		param := params.At(i)
		pType := param.Type()

		if pType.String() == contextType {
			continue
		}

		paramName := "_" + getParamName(param, i)

		paramNames = append(paramNames, paramName)

		switch pType.String() {
		case "string", "int", "bool", "error":
			w.Printf("\t\t%s := args.%s(%d)\n", paramName, strcase.ToPascal(pType.String()), pos)
		default:
			w.Printf("\t\t%s, _ := args.Get(%d).(%s)\n", paramName, pos, s.getTypeName(pType, false))
		}

		pos++
	}

	w.Printf("\t\tfn(%s", strings.Join(paramNames, ", "))
	if s.Signature.Variadic() {
		w.Print("...")
	}
	w.Println(")")

	w.Println("\t})")
	w.Println("\treturn _c")
	w.Println("}")
	w.Println()

	return w.Err()
}

func (s Syrup) returnsFn(writer io.Writer) error {
	w := &Writer{writer: writer}

	results := s.Signature.Results()
	if results.Len() < 1 {
		return nil
	}

	params := s.Signature.Params()

	structBaseName := strcase.ToGoCamel(s.InterfaceName)

	w.Printf("func (_c *%[1]s%[2]sCall%[4]s) ReturnsFn(fn %[3]s) *%[1]s%[2]sCall%[4]s {\n",
		structBaseName, s.Method.Name(), s.createFuncSignature(params, results), s.getTypeParamsUse())
	w.Println("\t_c.Call = _c.Return(fn)")
	w.Println("\treturn _c")
	w.Println("}")
	w.Println()

	return w.Err()
}

func (s Syrup) callMethodsOn(writer io.Writer, methods []*types.Func) error {
	w := &Writer{writer: writer}

	typeParamsUse := s.getTypeParamsUse()
	callType := fmt.Sprintf("%s%sCall%s", strcase.ToGoCamel(s.InterfaceName), s.Method.Name(), typeParamsUse)

	for _, method := range methods {
		sign := method.Type().(*types.Signature)

		w.Printf("func (_c *%s) On%s(", callType, method.Name())

		params := sign.Params()

		var argNames []string
		for i := range params.Len() {
			param := params.At(i)

			if param.Type().String() == contextType {
				continue
			}

			name := getParamName(param, i)

			w.Print(name)
			argNames = append(argNames, name)

			w.Print(" " + s.getTypeName(param.Type(), i == params.Len()-1))

			if i+1 < params.Len() {
				w.Print(", ")
			}
		}

		w.Printf(") *%s%sCall%s {\n", strcase.ToGoCamel(s.InterfaceName), method.Name(), typeParamsUse)

		w.Printf("\treturn _c.Parent.On%s(%s", method.Name(), strings.Join(argNames, ", "))
		if sign.Variadic() {
			w.Print("...")
		}
		w.Println(")")
		w.Println("}")
		w.Println()
	}

	return w.Err()
}

func (s Syrup) callMethodOnRaw(writer io.Writer, methods []*types.Func) error {
	w := &Writer{writer: writer}

	typeParamsUse := s.getTypeParamsUse()
	callType := fmt.Sprintf("%s%sCall%s", strcase.ToGoCamel(s.InterfaceName), s.Method.Name(), typeParamsUse)

	for _, method := range methods {
		sign := method.Type().(*types.Signature)

		w.Printf("func (_c *%s) On%sRaw(", callType, method.Name())

		params := sign.Params()

		var argNames []string
		for i := range params.Len() {
			param := params.At(i)

			if param.Type().String() == contextType {
				continue
			}

			name := getParamName(param, i)

			w.Print(name)
			argNames = append(argNames, name)

			w.Print(" interface{}")

			if i+1 < params.Len() {
				w.Print(", ")
			}
		}

		w.Printf(") *%s%sCall%s {\n", strcase.ToGoCamel(s.InterfaceName), method.Name(), typeParamsUse)

		w.Printf("\treturn _c.Parent.On%sRaw(%s)\n", method.Name(), strings.Join(argNames, ", "))
		w.Println("}")
		w.Println()
	}

	return w.Err()
}

func (s Syrup) getTypeName(t types.Type, last bool) string {
	switch v := t.(type) {
	case *types.Basic:
		return v.Name()

	case *types.Slice:
		if s.Signature.Variadic() && last {
			return "..." + s.getTypeName(v.Elem(), false)
		}

		return "[]" + s.getTypeName(v.Elem(), false)

	case *types.Map:
		return "map[" + s.getTypeName(v.Key(), false) + "]" + s.getTypeName(v.Elem(), false)

	case *types.Named:
		return s.getNamedTypeName(v)

	case *types.Pointer:
		return "*" + s.getTypeName(v.Elem(), false)

	case *types.Struct:
		return v.String()

	case *types.Interface:
		return v.String()

	case *types.Signature:
		fn := "func(" + strings.Join(s.getTupleTypes(v.Params()), ",") + ")"

		if v.Results().Len() > 0 {
			fn += " (" + strings.Join(s.getTupleTypes(v.Results()), ",") + ")"
		}

		return fn

	case *types.Chan:
		return s.getChanTypeName(v)

	case *types.Array:
		return fmt.Sprintf("[%d]%s", v.Len(), s.getTypeName(v.Elem(), false))

	case *types.TypeParam:
		return v.Obj().Name()

	default:
		panic(fmt.Sprintf("OOPS %[1]T %[1]s", t))
	}
}

func (s Syrup) getTupleTypes(t *types.Tuple) []string {
	var tupleTypes []string
	for i := range t.Len() {
		param := t.At(i)

		tupleTypes = append(tupleTypes, s.getTypeName(param.Type(), false))
	}

	return tupleTypes
}

func (s Syrup) getNamedTypeName(t *types.Named) string {
	if t.Obj() != nil && t.Obj().Pkg() != nil {
		if t.Obj().Pkg().Path() == s.PkgPath {
			return t.Obj().Name()
		}
		return t.Obj().Pkg().Name() + "." + t.Obj().Name()
	}

	name := t.String()

	i := strings.LastIndex(t.String(), "/")
	if i > -1 {
		name = name[i+1:]
	}
	return name
}

func (s Syrup) getChanTypeName(t *types.Chan) string {
	var typ string
	switch t.Dir() {
	case types.SendRecv:
		typ = "chan"
	case types.SendOnly:
		typ = "chan<-"
	case types.RecvOnly:
		typ = "<-chan"
	}

	return typ + " " + s.getTypeName(t.Elem(), false)
}

func (s Syrup) createFuncSignature(params, results *types.Tuple) string {
	fnSign := "func("
	for i := range params.Len() {
		param := params.At(i)
		if param.Type().String() == contextType {
			continue
		}

		fnSign += s.getTypeName(param.Type(), i == params.Len()-1)

		if i+1 < params.Len() {
			fnSign += ", "
		}
	}
	fnSign += ") "

	if results != nil {
		fnSign += "("
		for i := range results.Len() {
			rType := results.At(i).Type()
			fnSign += s.getTypeName(rType, false)
			if i+1 < results.Len() {
				fnSign += ", "
			}
		}
		fnSign += ")"
	}

	return fnSign
}

func writeImports(writer io.Writer, descPkg PackageDesc) error {
	base := template.New("templateImports")

	tmpl, err := base.Parse(templateImports)
	if err != nil {
		return err
	}

	data := map[string]interface{}{
		"Name":    descPkg.Pkg.Name(),
		"Imports": quickGoImports(descPkg),
	}
	return tmpl.Execute(writer, data)
}

func writeMockBase(writer io.Writer, interfaceDesc InterfaceDesc, exported bool) error {
	base := template.New("templateMockBase").Funcs(template.FuncMap{
		"ToGoCamel":  strcase.ToGoCamel,
		"ToGoPascal": strcase.ToGoPascal,
	})

	constructorPrefix := "new"
	if exported {
		constructorPrefix = "New"
	}

	// Generate type parameter declarations and usage
	typeParamsDecl := ""
	typeParamsUse := ""
	if interfaceDesc.TypeParams != nil && interfaceDesc.TypeParams.Len() > 0 {
		var params []string
		var names []string
		for i := range interfaceDesc.TypeParams.Len() {
			tp := interfaceDesc.TypeParams.At(i)
			params = append(params, tp.Obj().Name()+" "+tp.Constraint().String())
			names = append(names, tp.Obj().Name())
		}
		typeParamsDecl = "[" + strings.Join(params, ", ") + "]"
		typeParamsUse = "[" + strings.Join(names, ", ") + "]"
	}

	tmpl, err := base.Parse(templateMockBase)
	if err != nil {
		return err
	}

	data := map[string]interface{}{
		"InterfaceName":     interfaceDesc.Name,
		"ConstructorPrefix": constructorPrefix,
		"TypeParamsDecl":    typeParamsDecl,
		"TypeParamsUse":     typeParamsUse,
	}
	return tmpl.Execute(writer, data)
}

func quickGoImports(descPkg PackageDesc) []string {
	imports := []string{
		"", // to separate std imports than the others
	}

	descPkg.Imports["testing"] = struct{}{}                          // require by test
	descPkg.Imports["time"] = struct{}{}                             // require by `WaitUntil(w <-chan time.Time)`
	descPkg.Imports["github.com/stretchr/testify/mock"] = struct{}{} // require by mock

	for imp := range descPkg.Imports {
		imports = append(imports, imp)
	}

	sort.Slice(imports, func(i, j int) bool {
		if imports[i] == "" {
			return strings.Contains(imports[j], ".")
		}
		if imports[j] == "" {
			return !strings.Contains(imports[i], ".")
		}

		if strings.Contains(imports[i], ".") && !strings.Contains(imports[j], ".") {
			return false
		}
		if !strings.Contains(imports[i], ".") && strings.Contains(imports[j], ".") {
			return true
		}

		return imports[i] < imports[j]
	})

	return imports
}

func getParamName(tVar *types.Var, i int) string {
	if tVar.Name() == "" {
		return fmt.Sprintf("%sParam", string(rune('a'+i)))
	}
	return tVar.Name()
}

func getResultName(tVar *types.Var, i int) string {
	if tVar.Name() == "" {
		return fmt.Sprintf("_r%s%d", string(rune('a'+i)), i)
	}
	return tVar.Name()
}

// Writer is a wrapper around Print+ functions.
type Writer struct {
	writer io.Writer
	err    error
}

// Err returns error from the other methods.
func (w *Writer) Err() error {
	return w.err
}

// Print formats using the default formats for its operands and writes to standard output.
func (w *Writer) Print(a ...interface{}) {
	if w.err != nil {
		return
	}

	_, w.err = fmt.Fprint(w.writer, a...)
}

// Printf formats according to a format specifier and writes to standard output.
func (w *Writer) Printf(pattern string, a ...interface{}) {
	if w.err != nil {
		return
	}

	_, w.err = fmt.Fprintf(w.writer, pattern, a...)
}

// Println formats using the default formats for its operands and writes to standard output.
func (w *Writer) Println(a ...interface{}) {
	if w.err != nil {
		return
	}

	_, w.err = fmt.Fprintln(w.writer, a...)
}
