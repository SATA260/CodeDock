package seam

import (
	"context"
	"errors"
	"testing"
)

func TestDispatchNilReturnsOriginal(t *testing.T) {
	t.Parallel()
	in := Envelope{Type: TypeInput, Payload: []byte(`{"content":"hi"}`)}
	got, err := Dispatch(context.Background(), nil, in)
	if err != nil {
		t.Fatal(err)
	}
	if got.Type != in.Type || string(got.Payload) != string(in.Payload) {
		t.Fatalf("got=%+v", got)
	}
}

func TestFuncRewrites(t *testing.T) {
	t.Parallel()
	d := Func(func(_ context.Context, ev Envelope) (Envelope, error) {
		ev.Type = TypeInputHandled
		return ev, nil
	})
	got, err := Dispatch(context.Background(), d, Envelope{Type: TypeInput})
	if err != nil || got.Type != TypeInputHandled {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

func TestFuncNilAndError(t *testing.T) {
	t.Parallel()
	var empty Func
	got, err := Dispatch(context.Background(), empty, Envelope{Type: TypeRequest})
	if err != nil || got.Type != TypeRequest {
		t.Fatalf("nil func: %+v %v", got, err)
	}
	want := errors.New("boom")
	_, err = Dispatch(context.Background(), Func(func(context.Context, Envelope) (Envelope, error) {
		return Envelope{}, want
	}), Envelope{Type: TypeInput})
	if !errors.Is(err, want) {
		t.Fatalf("err=%v", err)
	}
}

func TestIsSeam(t *testing.T) {
	t.Parallel()
	if !IsSeam(TypeInput) || !IsSeam(TypePostExecute) {
		t.Fatal("seams should match")
	}
	if IsSeam(TypeInputHandled) || IsSeam("run.completed") || IsSeam("") {
		t.Fatal("non-seams should not match")
	}
}
