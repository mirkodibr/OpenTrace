package opentrace

import (
	"errors"
	"testing"
	"time"
)

func TestFieldConstructors(t *testing.T) {
	tests := []struct {
		name  string
		field Field
		check func(t *testing.T, f Field)
	}{
		{"String", String("k", "v"), func(t *testing.T, f Field) {
			if f.ftype != typeString || f.StringVal != "v" {
				t.Errorf("String field = %+v", f)
			}
		}},
		{"Int", Int("k", -7), func(t *testing.T, f Field) {
			if f.ftype != typeInt64 || f.Int64Val != -7 {
				t.Errorf("Int field = %+v", f)
			}
		}},
		{"Int64", Int64("k", 1<<40), func(t *testing.T, f Field) {
			if f.ftype != typeInt64 || f.Int64Val != 1<<40 {
				t.Errorf("Int64 field = %+v", f)
			}
		}},
		{"Float64", Float64("k", 3.5), func(t *testing.T, f Field) {
			if f.ftype != typeFloat64 || f.Float64Val != 3.5 {
				t.Errorf("Float64 field = %+v", f)
			}
		}},
		{"Bool", Bool("k", true), func(t *testing.T, f Field) {
			if f.ftype != typeBool || !f.BoolVal {
				t.Errorf("Bool field = %+v", f)
			}
		}},
		{"Duration", Duration("k", 1500*time.Millisecond), func(t *testing.T, f Field) {
			if f.ftype != typeDuration || f.Int64Val != int64(1500*time.Millisecond) {
				t.Errorf("Duration field = %+v", f)
			}
		}},
		{"Any", Any("k", struct{ X int }{X: 1}), func(t *testing.T, f Field) {
			if f.ftype != typeAny || f.Interface == nil {
				t.Errorf("Any field = %+v", f)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.field.Key != "k" {
				t.Errorf("Key = %q, want \"k\"", tt.field.Key)
			}
			tt.check(t, tt.field)
		})
	}
}

func TestErrField(t *testing.T) {
	sentinel := errors.New("boom")
	f := Err(sentinel)
	if f.Key != "error" || f.ftype != typeError {
		t.Errorf("Err field = %+v", f)
	}
	if f.StringVal != "boom" {
		t.Errorf("Err StringVal = %q, want \"boom\"", f.StringVal)
	}
	if !errors.Is(f.Interface.(error), sentinel) {
		t.Error("Err field lost the original error")
	}

	// nil error degrades to a string field, never panics.
	f = Err(nil)
	if f.Key != "error" || f.ftype != typeString || f.StringVal != "<nil>" {
		t.Errorf("Err(nil) field = %+v", f)
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		in     string
		want   Level
		wantOK bool
	}{
		{"debug", LevelDebug, true},
		{"info", LevelInfo, true},
		{"warn", LevelWarn, true},
		{"warning", LevelWarn, true},
		{"error", LevelError, true},
		{"fatal", LevelFatal, true},
		{"VERBOSE", LevelInfo, false},
		{"", LevelInfo, false},
	}
	for _, tt := range tests {
		got, ok := ParseLevel(tt.in)
		if got != tt.want || ok != tt.wantOK {
			t.Errorf("ParseLevel(%q) = (%v, %v), want (%v, %v)", tt.in, got, ok, tt.want, tt.wantOK)
		}
	}
}

func TestLevelString(t *testing.T) {
	for lvl, want := range map[Level]string{
		LevelDebug: "debug", LevelInfo: "info", LevelWarn: "warn",
		LevelError: "error", LevelFatal: "fatal", Level(99): "unknown",
	} {
		if got := lvl.String(); got != want {
			t.Errorf("Level(%d).String() = %q, want %q", lvl, got, want)
		}
	}
}
