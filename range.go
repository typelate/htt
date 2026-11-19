package htt

import "reflect"

// rangeOver iterates val as {{range}} does and calls each once per
// element. each reports stop to end the loop early. ran is false when val
// held nothing, which is when the else list runs.
func (s *state) rangeOver(val reflect.Value, vars int, each func(index, elem reflect.Value) (stop bool, err error)) (ran bool, err error) {
	// A nil interface holds nothing to range over, as a nil one does in
	// text/template.
	if val.Kind() == reflect.Interface && val.IsNil() {
		return false, nil
	}
	switch val.Kind() {
	case reflect.Array, reflect.Slice:
		if val.Len() == 0 {
			return false, nil
		}
		for i := 0; i < val.Len(); i++ {
			stop, err := each(reflect.ValueOf(i), val.Index(i))
			if err != nil {
				return false, err
			}
			if stop {
				break
			}
		}
		return true, nil
	case reflect.Map:
		if val.Len() == 0 {
			return false, nil
		}
		for _, entry := range sortMap(val) {
			stop, err := each(entry.key, entry.value)
			if err != nil {
				return false, err
			}
			if stop {
				break
			}
		}
		return true, nil
	case reflect.Chan:
		if val.IsNil() {
			return false, nil
		}
		if val.Type().ChanDir() == reflect.SendDir {
			return false, s.errorf("range over send-only channel %v", val)
		}
		received := false
		for i := 0; ; i++ {
			elem, ok := val.Recv()
			if !ok {
				return received, nil
			}
			received = true
			stop, err := each(reflect.ValueOf(i), elem)
			if err != nil {
				return false, err
			}
			if stop {
				return true, nil
			}
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if vars > 1 {
			return false, s.errorf("can't use %v to iterate over more than one variable", val)
		}
		count := val.Int()
		if count <= 0 {
			return false, nil
		}
		return rangeCount(uint64(count), val.Type(), each)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		if vars > 1 {
			return false, s.errorf("can't use %v to iterate over more than one variable", val)
		}
		count := val.Uint()
		if count == 0 {
			return false, nil
		}
		return rangeCount(count, val.Type(), each)
	case reflect.Func:
		if val.Type().CanSeq2() {
			ran := false
			for key, value := range val.Seq2() {
				ran = true
				var stop bool
				var err error
				if vars > 1 {
					stop, err = each(key, value)
				} else {
					stop, err = each(reflect.Value{}, key)
				}
				if err != nil {
					return false, err
				}
				if stop {
					break
				}
			}
			return ran, nil
		}
		if !val.Type().CanSeq() {
			return false, s.errorf("range can't iterate over %v", val)
		}
		if vars > 1 {
			return false, s.errorf("can't use %v iterate over more than one variable", val)
		}
		ran := false
		for elem := range val.Seq() {
			ran = true
			stop, err := each(reflect.Value{}, elem)
			if err != nil {
				return false, err
			}
			if stop {
				break
			}
		}
		return ran, nil
	case reflect.Invalid:
		// nil is like empty
		return false, nil
	default:
		return false, s.errorf("range can't iterate over %v", val)
	}
}

// rangeCount calls each with the indexes 0 through n-1 and elements of
// type typ. Callers rule out n == 0 first, because an empty range runs
// the else list.
func rangeCount(n uint64, typ reflect.Type, each func(index, elem reflect.Value) (stop bool, err error)) (bool, error) {
	for i := uint64(0); i < n; i++ {
		stop, err := each(reflect.ValueOf(int(i)), reflect.ValueOf(i).Convert(typ))
		if err != nil {
			return false, err
		}
		if stop {
			break
		}
	}
	return true, nil
}

// setRangeVars gives the range's declared or assigned variables the
// values of one iteration.
func (s *state) setRangeVars(pipe *PipeNode, index, elem reflect.Value) error {
	if len(pipe.Decl) == 0 {
		return nil
	}
	if pipe.IsAssign {
		if len(pipe.Decl) > 1 {
			if err := s.setVar(pipe.Decl[0].Ident[0], index); err != nil {
				return err
			}
		}
		return s.setVar(pipe.Decl[len(pipe.Decl)-1].Ident[0], elem)
	}
	s.setTopVar(1, elem)
	if len(pipe.Decl) > 1 {
		s.setTopVar(2, index)
	}
	return nil
}
