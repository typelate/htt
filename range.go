package htt

import "reflect"

// rangeOver iterates val as {{range}} does and calls each once per
// element. each reports stop to end the loop early. ran is false when val
// held nothing, which is when the else list runs.
func (s *state) rangeOver(val reflect.Value, each func(index, elem reflect.Value) (stop bool, err error)) (ran bool, err error) {
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
		iter := val.MapRange()
		for iter.Next() {
			stop, err := each(iter.Key(), iter.Value())
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
		count := val.Int()
		if count <= 0 {
			return false, nil
		}
		for i := int64(0); i < count; i++ {
			stop, err := each(reflect.ValueOf(int(i)), reflect.ValueOf(int(i)))
			if err != nil {
				return false, err
			}
			if stop {
				break
			}
		}
		return true, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		count := val.Uint()
		if count == 0 {
			return false, nil
		}
		for i := uint64(0); i < count; i++ {
			stop, err := each(reflect.ValueOf(int(i)), reflect.ValueOf(int(i)))
			if err != nil {
				return false, err
			}
			if stop {
				break
			}
		}
		return true, nil
	case reflect.Invalid:
		// nil is like empty
		return false, nil
	default:
		return false, s.errorf("range can't iterate over %v", val)
	}
}

// setRangeVars gives the range's declared or assigned variables the
// values of one iteration.
func (s *state) setRangeVars(pipe *PipeNode, index, elem reflect.Value) {
	if len(pipe.Decl) == 0 {
		return
	}
	if pipe.IsAssign {
		if len(pipe.Decl) > 1 {
			s.setVar(pipe.Decl[0].Ident[0], index)
		}
		s.setVar(pipe.Decl[len(pipe.Decl)-1].Ident[0], elem)
		return
	}
	s.setTopVar(1, elem)
	if len(pipe.Decl) > 1 {
		s.setTopVar(2, index)
	}
}
