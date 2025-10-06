package logger

import (
	"bytes"
	"strings"
	"testing"
)

func TestLogger_BasicFunctionality(t *testing.T) {
	var buf bytes.Buffer
	l := NewLogger(false)
	l.SetOutput(&buf)

	l.Info("test message")
	if !strings.Contains(buf.String(), "test message") {
		t.Errorf("Expected message to contain 'test message', got: %s", buf.String())
	}
}

func TestLogger_VerboseMode(t *testing.T) {
	var buf bytes.Buffer
	l := NewLogger(false)
	l.SetOutput(&buf)

	l.Debug("debug message")
	if buf.String() != "" {
		t.Errorf("Debug message should not appear when verbose=false, got: %s", buf.String())
	}

	buf.Reset()
	l.SetVerbose(true)
	l.Debug("debug message")
	if !strings.Contains(buf.String(), "debug message") {
		t.Errorf("Debug message should appear when verbose=true, got: %s", buf.String())
	}
}

func TestLogger_Warning(t *testing.T) {
	var buf bytes.Buffer
	l := NewLogger(false)
	l.SetOutput(&buf)

	l.Warn("warning message")
	output := buf.String()
	if !strings.Contains(output, "Warning:") || !strings.Contains(output, "warning message") {
		t.Errorf("Expected warning with prefix, got: %s", output)
	}
}

func TestLogger_Error(t *testing.T) {
	var buf bytes.Buffer
	l := NewLogger(false)
	l.SetOutput(&buf)

	l.Error("error message")
	output := buf.String()
	if !strings.Contains(output, "Error:") || !strings.Contains(output, "error message") {
		t.Errorf("Expected error with prefix, got: %s", output)
	}
}

func TestDefaultLogger(t *testing.T) {
	original := Default()

	var buf bytes.Buffer
	newLogger := NewLogger(false)
	newLogger.SetOutput(&buf)
	SetDefault(newLogger)

	Info("test info")
	if !strings.Contains(buf.String(), "test info") {
		t.Errorf("Expected default logger to be used, got: %s", buf.String())
	}

	SetDefault(original)
}

func TestGlobalFunctions(t *testing.T) {
	var buf bytes.Buffer
	l := NewLogger(false)
	l.SetOutput(&buf)
	SetDefault(l)

	Info("global info")
	if !strings.Contains(buf.String(), "global info") {
		t.Errorf("Global Info function failed, got: %s", buf.String())
	}

	buf.Reset()
	Warn("global warn")
	if !strings.Contains(buf.String(), "global warn") {
		t.Errorf("Global Warn function failed, got: %s", buf.String())
	}

	buf.Reset()
	Errorf("global error")
	if !strings.Contains(buf.String(), "global error") {
		t.Errorf("Global Errorf function failed, got: %s", buf.String())
	}
}

func TestSetOutput_Global(t *testing.T) {
	var buf bytes.Buffer
	SetOutput(&buf)

	Info("output test")
	if !strings.Contains(buf.String(), "output test") {
		t.Errorf("Global SetOutput failed, got: %s", buf.String())
	}
}

func TestSetInfoOutput(t *testing.T) {
	var infoBuf bytes.Buffer
	var errBuf bytes.Buffer

	l := NewLogger(false)
	l.SetInfoOutput(&infoBuf)
	l.SetErrorOutput(&errBuf)

	l.Info("info message")
	l.Warn("warn message")

	if !strings.Contains(infoBuf.String(), "info message") {
		t.Errorf("Info message should go to info output, got: %s", infoBuf.String())
	}
	if !strings.Contains(errBuf.String(), "warn message") {
		t.Errorf("Warn message should go to error output, got: %s", errBuf.String())
	}
	if strings.Contains(infoBuf.String(), "warn") {
		t.Errorf("Warn message should not go to info output")
	}
}

func TestConcurrency(t *testing.T) {
	var buf bytes.Buffer
	l := NewLogger(false)
	l.SetOutput(&buf)

	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(n int) {
			l.SetVerbose(n%2 == 0)
			l.Info("message %d", n)
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	output := buf.String()
	if !strings.Contains(output, "message") {
		t.Error("Expected concurrent logging to work")
	}
}
