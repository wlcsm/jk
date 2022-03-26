package main

const (
	HL_HIGHLIGHT_NUMBERS = 1 << iota
	HL_HIGHLIGHT_STRINGS
)

type EditorSyntax struct {
	// Name of the filetype displayed in the status bar.
	filetype string
	// List of patterns to match a filename against.
	filematch []string
	// List of keywords to highlight.
	keywords []string
	// Second highlight group
	keywords2 []string
	// scs is a single-line comment start pattern (e.g. "//" for golang).
	// set to an empty string if comment highlighting is not needed.
	scs string
	// mcs is a multi-line comment start pattern (e.g. "/*" for golang).
	mcs string
	// mce is a multi-line comment end pattern (e.g. "*/" for golang).
	mce string

	highlightStrings bool
	highlightNumbers bool
}

var HLDB = []*EditorSyntax{
	{
		filetype:  "c",
		filematch: []string{".c", ".h", "cpp", ".cc"},
		keywords: []string{
			"switch", "if", "while", "for", "break", "continue", "return",
			"else", "struct", "union", "typedef", "static", "enum", "class",
			"case"},
		keywords2: []string{
			"int", "long", "double", "float", "char", "unsigned",
			"signed", "void",
		},
		scs:   "//",
		mcs:   "/*",
		mce:   "*/",
		highlightStrings: true,
		highlightNumbers: true,
	},
	{
		filetype:  "go",
		filematch: []string{".go"},
		keywords: []string{
			"break", "default", "func", "interface", "select", "case", "defer",
			"go", "map", "struct", "chan", "else", "goto", "package", "switch",
			"const", "fallthrough", "if", "range", "type", "continue", "for",
			"import", "return", "var"},

		keywords2: []string{
			"append", "bool", "byte", "cap", "close", "complex",
			"complex64", "complex128", "error", "uint16", "copy", "false",
			"float32", "float64", "imag", "int", "int8", "int16",
			"uint32", "int32", "int64", "iota", "len", "make", "new",
			"nil", "panic", "uint64", "print", "println", "real",
			"recover", "rune", "string", "true", "uint", "uint8",
			"uintptr",
		},
		scs:   "//",
		mcs:   "/*",
		mce:   "*/",
		highlightStrings: true,
		highlightNumbers: true,
	},
}
