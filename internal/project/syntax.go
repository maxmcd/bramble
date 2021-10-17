package project

import (
	"fmt"
	"strings"

	"go.starlark.net/syntax"
)

func printDef(def *syntax.DefStmt) string {
	return fmt.Sprintf("def %s(%s)", def.Name.Name, strings.Join(argumentsToStringList(def.Params), ", "))
}

func argumentsToStringList(args []syntax.Expr) []string {
	params := make([]string, 0, len(args))
	for _, param := range args {
		params = append(params, valToString(param))
	}
	return params
}

// TODO: would be fun to do this the iterative way not the recursive way and
// benchmark them
func valToString(expr syntax.Expr) string {
	switch v := expr.(type) {
	case *syntax.Ident:
		return v.Name
	case *syntax.Literal:
		return v.Raw
	case *syntax.CallExpr:
		return fmt.Sprintf("%s(%s)", valToString(v.Fn), strings.Join(argumentsToStringList(v.Args), ", "))
	case *syntax.DictExpr:
		return fmt.Sprintf("{%s}", strings.Join(argumentsToStringList(v.List), ", "))
	case *syntax.DictEntry:
		return fmt.Sprintf("%s: %s", valToString(v.Key), valToString(v.Value))
	case *syntax.ListExpr:
		items := make([]string, 0, len(v.List))
		for _, item := range v.List {
			items = append(items, valToString(item))
		}
		return fmt.Sprintf("[%s]", strings.Join(items, ", "))
	case *syntax.BinaryExpr:
		return fmt.Sprintf("%s%s%s", valToString(v.X), v.Op.String(), valToString(v.Y))
	}
	return ""
}

func (p *Project) parseStarlarkFile(file *syntax.File) (m ModuleDoc) {
	m.Docstring = docStringFromBody(file.Stmts)

	for _, stmt := range file.Stmts {
		if def, ok := stmt.(*syntax.DefStmt); ok {
			if strings.HasPrefix(def.Name.Name, "_") {
				continue
			}
			m.Functions = append(m.Functions, FunctionDoc{
				Definition: printDef(def),
				Name:       def.Name.Name,
				Docstring:  strings.TrimSpace(docStringFromBody(def.Body)),
			})
		}
	}
	return
}

func docStringFromBody(body []syntax.Stmt) string {
	if len(body) == 0 {
		return ""
	}
	expr, ok := body[0].(*syntax.ExprStmt)
	if !ok {
		return ""
	}
	lit, ok := expr.X.(*syntax.Literal)
	if !ok {
		return ""
	}
	if lit.Token != syntax.STRING {
		return ""
	}
	return lit.Raw
	// return lit.Value.(string)
}
