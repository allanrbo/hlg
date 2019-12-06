package main

const ringBufferInitialSize = 1000

type ringbuffer struct {
	buf  []int
	head int
	size int
}

func (r *ringbuffer) get() (val int, ok bool) {
	if r.size > 0 {
		val = r.buf[r.head]
		r.buf[r.head] = 0
		r.head = (r.head + 1) % len(r.buf)
		r.size--
		ok = true
	}

	return
}

func (r *ringbuffer) put(val int) {
	if len(r.buf) == 0 {
		// First call to put.
		r.buf = make([]int, ringBufferInitialSize)
	}

	if r.size == len(r.buf) {
		// Make a new ring buffer 50% bigger.
		newBuf := make([]int, int(float64(len(r.buf))*1.5))

		// Copy from old ring buffer to new.
		var i int
		cur := r.head
		for {
			if i == r.size {
				break
			}
			newBuf[i] = r.buf[cur]
			i++
			cur = (cur + 1) % len(r.buf)
		}
		r.buf = newBuf
		r.head = 0
	}

	r.buf[(r.head+r.size)%len(r.buf)] = val
	r.size++

	return
}

func (r *ringbuffer) remove(val int) {
	var checkedCount int
	cur := r.head
	for {
		if checkedCount == r.size {
			// We checked all elements, and we did not find val.
			break
		}
		checkedCount++

		if r.buf[cur] == val {
			// Element to remove was found. Remove item by moving all subsequent items one back.
			for {
				next := (cur + 1) % len(r.buf)
				r.buf[cur] = r.buf[next]

				if checkedCount == r.size {
					r.buf[cur] = 0
					break
				}
				checkedCount++

				cur = (cur + 1) % len(r.buf)
			}

			r.size--
			break
		}

		cur = (cur + 1) % len(r.buf)
	}

	return
}
