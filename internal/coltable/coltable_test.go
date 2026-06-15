package coltable

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParse_BasicWithPreamble(t *testing.T) {
	raw := "starting some banner\n" +
		"NAME    VALUE   NOTE\n" +
		"alpha   1       first\n" +
		"beta    2       second\n"

	rows, err := Parse(raw, []string{"NAME", "VALUE", "NOTE"})
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, map[string]string{"NAME": "alpha", "VALUE": "1", "NOTE": "first"}, rows[0])
	require.Equal(t, map[string]string{"NAME": "beta", "VALUE": "2", "NOTE": "second"}, rows[1])
}

func TestParse_ShortLineLeavesTrailingColumnsEmpty(t *testing.T) {
	// A continuation-style line that only reaches the last column: leading columns
	// are blank, the final column carries the value.
	raw := "NAME    VALUE   NOTE\n" +
		"alpha   1       first\n" +
		"                extra\n"

	rows, err := Parse(raw, []string{"NAME", "VALUE", "NOTE"})
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, "", rows[1]["NAME"])
	require.Equal(t, "", rows[1]["VALUE"])
	require.Equal(t, "extra", rows[1]["NOTE"])
}

func TestParse_BlankLinesSkipped(t *testing.T) {
	raw := "NAME    VALUE   NOTE\n" +
		"alpha   1       first\n" +
		"\n" +
		"beta    2       second\n"

	rows, err := Parse(raw, []string{"NAME", "VALUE", "NOTE"})
	require.NoError(t, err)
	require.Len(t, rows, 2)
}

func TestParse_NoHeader(t *testing.T) {
	rows, err := Parse(`No secrets found for scope "x".`, []string{"NAME", "VALUE", "NOTE"})
	require.ErrorIs(t, err, ErrNoHeader)
	require.Nil(t, rows)
}

func TestParse_EmptyInput(t *testing.T) {
	rows, err := Parse("", []string{"NAME", "VALUE"})
	require.ErrorIs(t, err, ErrNoHeader)
	require.Nil(t, rows)
}

func TestParse_HeaderMismatchRenamed(t *testing.T) {
	// Header-like row (uppercase first token, >=2 columns) but a renamed column.
	raw := "NAME    AMOUNT  NOTE\n" +
		"alpha   1       first\n"

	rows, err := Parse(raw, []string{"NAME", "VALUE", "NOTE"})
	require.ErrorIs(t, err, ErrHeaderMismatch)
	require.Nil(t, rows)
}

func TestParse_HeaderMismatchExtraColumn(t *testing.T) {
	raw := "NAME    VALUE   NOTE    EXTRA\n" +
		"alpha   1       first   x\n"

	_, err := Parse(raw, []string{"NAME", "VALUE", "NOTE"})
	require.ErrorIs(t, err, ErrHeaderMismatch)
}

func TestParse_ShortRowMidColumnEmpty(t *testing.T) {
	// A row that stops before the VALUE column's offset: NAME present, the rest empty.
	raw := "NAME    VALUE   NOTE\n" +
		"ab\n"
	rows, err := Parse(raw, []string{"NAME", "VALUE", "NOTE"})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "ab", rows[0]["NAME"])
	require.Equal(t, "", rows[0]["VALUE"])
	require.Equal(t, "", rows[0]["NOTE"])
}

func TestParse_SingleColumnLabelIsNotHeader(t *testing.T) {
	// A lone uppercase label like a section title must not be taken as a header.
	rows, err := Parse("CUSTOM SECRETS\n", []string{"NAME", "VALUE"})
	require.ErrorIs(t, err, ErrNoHeader)
	require.Nil(t, rows)
}
