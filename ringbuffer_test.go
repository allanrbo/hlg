package main

import "testing"

func TestRingBufferEmpty(t *testing.T) {
	// Arrange
	rb := ringbuffer{}

	// Act
	_, ok := rb.get()

	// Assert
	if ok {
		t.Fatalf("Unexpected ok from rb.get()")
	}
}

func TestRingBufferSimple(t *testing.T) {
	// Arrange
	rb := ringbuffer{}

	// Act
	rb.put(10)
	rb.put(20)
	rb.put(30)
	r1, ok1 := rb.get()
	r2, ok2 := rb.get()
	r3, ok3 := rb.get()
	_, ok4 := rb.get()

	// Assert
	if r1 != 10 {
		t.Fatalf("Unexpected r1")
	}
	if r2 != 20 {
		t.Fatalf("Unexpected r2")
	}
	if r3 != 30 {
		t.Fatalf("Unexpected r3")
	}
	if !ok1 {
		t.Fatalf("Unexpected ok1")
	}
	if !ok2 {
		t.Fatalf("Unexpected ok1")
	}
	if !ok3 {
		t.Fatalf("Unexpected ok1")
	}
	if ok4 {
		t.Fatalf("Unexpected ok4")
	}
}

func TestRingBufferWrapAroundRingBuffer(t *testing.T) {
	// Arrange
	rb := ringbuffer{}
	var expectedVals []int

	// Act
	for i := 0; i < ringBufferInitialSize; i++ {
		val := i * 10
		expectedVals = append(expectedVals, val)
		rb.put(val)
	}
	for i := 0; i < 2; i++ {
		// Free some elements from the front so we can put more in place of these.
		val1, ok1 := rb.get()
		if !ok1 {
			t.Fatalf("Unexpected ok1")
		}
		if val1 != expectedVals[0] {
			t.Fatalf("Unexpected val1 %v", val1)
		}
		expectedVals = expectedVals[1:]
	}
	for i := 0; i < 2; i++ {
		val := i * 1000
		expectedVals = append(expectedVals, val)
		rb.put(val)
	}

	// Assert
	for _, expectedVal := range expectedVals {
		val2, ok2 := rb.get()
		if !ok2 {
			t.Fatalf("Unexpected ok2")
		}
		if val2 != expectedVal {
			t.Fatalf("Unexpected val2 %v", val2)
		}
	}
}

func TestRingBufferRemove(t *testing.T) {
	// Arrange
	rb := ringbuffer{}
	var expectedVals []int

	// Act
	for i := 0; i < 7; i++ {
		val := i * 10
		expectedVals = append(expectedVals, val)
		rb.put(val)
	}

	rb.remove(50)
	copy(expectedVals[5:], expectedVals[5+1:])
	expectedVals = expectedVals[:len(expectedVals)-1]

	// Assert
	for _, expectedVal := range expectedVals {
		val, ok := rb.get()
		if !ok {
			t.Fatalf("Unexpected ok")
		}
		if val != expectedVal {
			t.Fatalf("Unexpected val %v", val)
		}
	}
}

func TestRingBufferRemoveWrap(t *testing.T) {
	for elementIdxToRemove := 0; elementIdxToRemove < ringBufferInitialSize; elementIdxToRemove++ {
		// Arrange
		rb := ringbuffer{}
		var expectedVals []int

		// Act
		for i := 0; i < ringBufferInitialSize; i++ {
			val := i * 10
			expectedVals = append(expectedVals, val)
			rb.put(val)
		}
		for i := 0; i < 2; i++ {
			// Free some elements from the front so we can put more in place of these.
			val1, ok1 := rb.get()
			if !ok1 {
				t.Fatalf("Unexpected ok1")
			}
			if val1 != expectedVals[0] {
				t.Fatalf("Unexpected val1 %v", val1)
			}
			expectedVals = expectedVals[1:]
		}
		for i := 0; i < 2; i++ {
			val := i * 10
			expectedVals = append(expectedVals, val)
			rb.put(val)
		}

		rb.remove(expectedVals[elementIdxToRemove])
		copy(expectedVals[elementIdxToRemove:], expectedVals[elementIdxToRemove+1:])
		expectedVals = expectedVals[:len(expectedVals)-1]

		// Assert
		for _, expectedVal := range expectedVals {
			val2, ok2 := rb.get()
			if !ok2 {
				t.Fatalf("Unexpected ok2")
			}
			if val2 != expectedVal {
				t.Fatalf("Unexpected val2 %v", val2)
			}
		}
	}
}

func TestRingBufferRingBufferExpand(t *testing.T) {
	// Arrange
	rb := ringbuffer{}
	var expectedVals []int

	// Act
	for i := 0; i < ringBufferInitialSize; i++ {
		val := i * 10
		expectedVals = append(expectedVals, val)
		rb.put(val)
	}
	for i := 0; i < 2; i++ {
		// Free some elements from the front so we can put more in place of these.
		val1, ok1 := rb.get()
		if !ok1 {
			t.Fatalf("Unexpected ok1")
		}
		if val1 != expectedVals[0] {
			t.Fatalf("Unexpected val1 %v", val1)
		}
		expectedVals = expectedVals[1:]
	}
	for i := 0; i < 10; i++ {
		// Add more elements than the initial capacity to ensure the ring buffer auto expands
		val := i * 1000
		expectedVals = append(expectedVals, val)
		rb.put(val)
	}

	// Assert
	// Assert
	for _, expectedVal := range expectedVals {
		val2, ok2 := rb.get()
		if !ok2 {
			t.Fatalf("Unexpected ok2")
		}
		if val2 != expectedVal {
			t.Fatalf("Unexpected val2 %v", val2)
		}
	}
}
