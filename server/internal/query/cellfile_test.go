package query

import "testing"

func TestNewCellFile_Bytes(t *testing.T) {
	f := NewCellFile([]byte{1, 2, 3})
	if f.ContentType != "application/octet-stream" || f.Extension != ".bin" {
		t.Errorf("got %+v, want octet-stream/.bin", f)
	}
	if len(f.Data) != 3 {
		t.Errorf("Data = %v, want 3 bytes", f.Data)
	}
}

func TestNewCellFile_String(t *testing.T) {
	f := NewCellFile("hello")
	if f.ContentType != "text/plain; charset=utf-8" || f.Extension != ".txt" {
		t.Errorf("got %+v, want text/plain/.txt", f)
	}
	if string(f.Data) != "hello" {
		t.Errorf("Data = %q, want %q", f.Data, "hello")
	}
}

func TestNewCellFile_Nil(t *testing.T) {
	f := NewCellFile(nil)
	if f.Data != nil {
		t.Errorf("Data = %v, want nil", f.Data)
	}
}

func TestNewCellFile_OtherScalar(t *testing.T) {
	f := NewCellFile(int64(42))
	if string(f.Data) != "42" {
		t.Errorf("Data = %q, want %q", f.Data, "42")
	}
}
