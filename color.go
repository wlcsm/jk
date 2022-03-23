package main

type SyntaxHL uint8

// Syntax highlight enums
const (
	hlNormal SyntaxHL = iota + 1
	hlComment
	hlMlComment
	hlKeyword1
	hlKeyword2
	hlString
	hlNumber
	hlMatch
)

var defaultColorscheme = map[SyntaxHL]int{
	hlComment:   90,
	hlMlComment: 90,
	hlKeyword1:  94,
	hlKeyword2:  96,
	hlString:    36,
	hlNumber:    33,
	hlMatch:     32,
}

func SyntaxToColor(hl SyntaxHL) int {
	color, ok := defaultColorscheme[hl]
	if !ok {
		return 37
	}

	return color
}
