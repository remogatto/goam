package lib

import "fmt"
import "testing"

func TestHello(t *testing.T) {
	if fmt.Sprint(Hello) != "hello" {
		t.Fail()
	}
}

func TestGoodbye(t *testing.T) {
	if fmt.Sprint(Goodbye) != "goodbye" {
		t.Fail()
	}
}

func BenchmarkHello(b *testing.B) {
	for i := 0; i < b.N; i++ {
		fmt.Sprintf("%s", Hello)
	}
}

func BenchmarkGoodbye(b *testing.B) {
	for i := 0; i < b.N; i++ {
		fmt.Sprintf("%s", Goodbye)
	}
}
