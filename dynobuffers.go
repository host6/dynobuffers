/*
 * Copyright (c) 2018-present unTill Pro, Ltd. and Contributors
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

package dynobuffers

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"unicode"
	"unsafe"

	flatbuffers "github.com/google/flatbuffers/go"
	"gopkg.in/yaml.v2"
)

// FieldType s.e.
type FieldType int

const (
	// FieldTypeUnspecified - wrong type
	FieldTypeUnspecified FieldType = iota
	// FieldTypeObject field is nested Scheme
	FieldTypeObject
	// FieldTypeInt int32
	FieldTypeInt
	// FieldTypeLong int64
	FieldTypeLong
	// FieldTypeFloat float32
	FieldTypeFloat
	// FieldTypeDouble float64
	FieldTypeDouble
	// FieldTypeString variable length
	FieldTypeString
	// FieldTypeBool s.e.
	FieldTypeBool
	// FieldTypeByte byte
	FieldTypeByte
)

// storeObjectsAsBytes defines if nested objects will be stored as byte vectors.
var storeObjectsAsBytes = false

var yamlFieldTypesMap = map[string]FieldType{
	"int":    FieldTypeInt,
	"long":   FieldTypeLong,
	"float":  FieldTypeFloat,
	"double": FieldTypeDouble,
	"string": FieldTypeString,
	"bool":   FieldTypeBool,
	"byte":   FieldTypeByte,
	"":       FieldTypeObject,
}

// Buffer is wrapper for FlatBuffers
type Buffer struct {
	Scheme         *Scheme
	modifiedFields []*modifiedField
	tab            flatbuffers.Table
	isModified     bool
	owner          *Buffer
}

// Field describes a Scheme field
type Field struct {
	Name        string
	Ft          FieldType
	order       int
	IsMandatory bool
	FieldScheme *Scheme // != nil for FieldTypeObject only
	ownerScheme *Scheme
	IsArray     bool
}

type modifiedField struct {
	value    interface{}
	isAppend bool
}

// ObjectArray used to iterate over array of nested objects
type ObjectArray struct {
	Buffer  *Buffer
	Len     int
	curElem int
	start   flatbuffers.UOffsetT
}

// Next proceeds to a next nested object in the array. If true then .Buffer represents the next element
func (oa *ObjectArray) Next() bool {
	oa.curElem++
	if oa.curElem >= oa.Len {
		return false
	}
	oa.Buffer.tab.Pos = oa.Buffer.tab.Indirect(oa.start + flatbuffers.UOffsetT(oa.curElem)*flatbuffers.SizeUOffsetT)
	return true
}

// Value returns *dynobuffers.Buffer instance as current element
func (oa *ObjectArray) Value() interface{} {
	return oa.Buffer
}

func (b *Buffer) getAllValues(start flatbuffers.UOffsetT, arrLen int, f *Field) interface{} {
	bytesSlice := b.tab.Bytes[start:]
	switch f.Ft {
	case FieldTypeInt:
		src := *(*[]int32)(unsafe.Pointer(&bytesSlice))
		src = src[:arrLen]
		res := make([]int32, len(src))
		copy(res, src)
		return res
	case FieldTypeFloat:
		src := *(*[]float32)(unsafe.Pointer(&bytesSlice))
		src = src[:arrLen]
		res := make([]float32, len(src))
		copy(res, src)
		return res
	case FieldTypeDouble:
		src := *(*[]float64)(unsafe.Pointer(&bytesSlice))
		src = src[:arrLen]
		res := make([]float64, len(src))
		copy(res, src)
		return res
	case FieldTypeByte:
		return b.tab.Bytes[start : arrLen+int(start)]
	case FieldTypeBool:
		src := *(*[]bool)(unsafe.Pointer(&bytesSlice))
		src = src[:arrLen]
		res := make([]bool, len(src))
		copy(res, src)
		return res
	case FieldTypeLong:
		src := *(*[]int64)(unsafe.Pointer(&bytesSlice))
		src = src[:arrLen]
		res := make([]int64, len(src))
		copy(res, src)
		return res
	default:
		res := make([]string, arrLen)
		for i := 0; i < arrLen; i++ {
			res[i] = b.getByField(f, i).(string)
		}
		return res
	}
}

// Scheme describes fields and theirs order in byte array
type Scheme struct {
	Name      string
	FieldsMap map[string]*Field
	Fields    []*Field
}

// NewBuffer creates new empty Buffer
func NewBuffer(Scheme *Scheme) *Buffer {
	if Scheme == nil {
		panic("nil Scheme provided")
	}
	b := &Buffer{}
	b.Scheme = Scheme
	return b
}

// GetInt returns int32 value by name and if the Scheme contains the field and the value was set to non-nil
func (b *Buffer) GetInt(name string) (int32, bool) {
	o := b.getFieldUOffsetT(name)
	if o != 0 {
		return b.tab.GetInt32(o), true
	}
	return int32(0), false
}

// GetFloat returns float32 value by name and if the Scheme contains the field and if the value was set to non-nil
func (b *Buffer) GetFloat(name string) (float32, bool) {
	o := b.getFieldUOffsetT(name)
	if o != 0 {
		return b.tab.GetFloat32(o), true
	}
	return float32(0), false
}

// GetString returns string value by name and if the Scheme contains the field and if the value was set to non-nil
func (b *Buffer) GetString(name string) (string, bool) {
	o := b.getFieldUOffsetT(name)
	if o != 0 {
		return byteSliceToString(b.tab.ByteVector(o)), true
	}
	return "", false
}

// GetLong returns int64 value by name and if the Scheme contains the field and if the value was set to non-nil
func (b *Buffer) GetLong(name string) (int64, bool) {
	o := b.getFieldUOffsetT(name)
	if o != 0 {
		return b.tab.GetInt64(o), true
	}
	return int64(0), false
}

// GetDouble returns float64 value by name and if the Scheme contains the field and if the value was set to non-nil
func (b *Buffer) GetDouble(name string) (float64, bool) {
	o := b.getFieldUOffsetT(name)
	if o != 0 {
		return b.tab.GetFloat64(o), true
	}
	return float64(0), false
}

// GetByte returns byte value by name and if the Scheme contains the field and if the value was set to non-nil
func (b *Buffer) GetByte(name string) (byte, bool) {
	o := b.getFieldUOffsetT(name)
	if o != 0 {
		return b.tab.GetByte(o), true
	}
	return byte(0), false
}

// GetBool returns bool value by name and if the Scheme contains the field and if the value was set to non-nil
func (b *Buffer) GetBool(name string) (bool, bool) {
	o := b.getFieldUOffsetT(name)
	if o != 0 {
		return b.tab.GetBool(o), true
	}
	return false, false
}

func (b *Buffer) getFieldUOffsetT(name string) flatbuffers.UOffsetT {
	if len(b.tab.Bytes) == 0 {
		return 0
	}
	if f, ok := b.Scheme.FieldsMap[name]; ok {
		return b.getFieldUOffsetTByOrder(f.order)
	}
	return 0
}

func (b *Buffer) getFieldUOffsetTByOrder(order int) flatbuffers.UOffsetT {
	if len(b.tab.Bytes) == 0 {
		return 0
	}
	preOffset := flatbuffers.UOffsetT(b.tab.Offset(flatbuffers.VOffsetT((order + 2) * 2)))
	if preOffset == 0 {
		return 0
	}
	return preOffset + b.tab.Pos
}

func (b *Buffer) getByStringField(f *Field) (string, bool) {
	o := b.getFieldUOffsetTByOrder(f.order)
	if o == 0 {
		return "", false
	}
	return byteSliceToString(b.tab.ByteVector(o)), true
}

func (b *Buffer) getByField(f *Field, index int) interface{} {
	uOffsetT := b.getFieldUOffsetTByOrder(f.order)
	if uOffsetT == 0 {
		return nil
	}
	return b.getByUOffsetT(f, index, uOffsetT)
}

func (b *Buffer) getByUOffsetT(f *Field, index int, uOffsetT flatbuffers.UOffsetT) interface{} {
	if f.IsArray {
		arrayLen := b.tab.VectorLen(uOffsetT - b.tab.Pos)
		elemSize := getFBFieldSize(f.Ft)
		if isFixedSizeField(f) {
			// arrays with fixed-size elements are stored as byte arrays
			arrayLen = arrayLen / elemSize
		}
		if index < 0 {
			if f.Ft == FieldTypeObject {
				arr := &ObjectArray{Buffer: &Buffer{}}
				arr.Len = b.tab.VectorLen(uOffsetT - b.tab.Pos)
				arr.Buffer.tab.Bytes = b.tab.Bytes
				arr.start = b.tab.Vector(uOffsetT - b.tab.Pos)
				arr.curElem = -1
				arr.Buffer.Scheme = f.FieldScheme
				return arr
			}
			uOffsetT = b.tab.Vector(uOffsetT - b.tab.Pos)
			return b.getAllValues(uOffsetT, arrayLen, f)
		}
		if index > arrayLen-1 {
			return nil
		}
		uOffsetT = b.tab.Vector(uOffsetT-b.tab.Pos) + flatbuffers.UOffsetT(index*elemSize)
	}
	return b.getValueByUOffsetT(f, uOffsetT)
}

func (b *Buffer) getValueByUOffsetT(f *Field, uOffsetT flatbuffers.UOffsetT) interface{} {
	switch f.Ft {
	case FieldTypeInt:
		return b.tab.GetInt32(uOffsetT)
	case FieldTypeLong:
		return b.tab.GetInt64(uOffsetT)
	case FieldTypeFloat:
		return b.tab.GetFloat32(uOffsetT)
	case FieldTypeDouble:
		return b.tab.GetFloat64(uOffsetT)
	case FieldTypeByte:
		return b.tab.GetByte(uOffsetT)
	case FieldTypeBool:
		return b.tab.GetBool(uOffsetT)
	case FieldTypeObject:
		var res *Buffer
		if storeObjectsAsBytes {
			bytesNested := b.tab.ByteVector(uOffsetT)
			res = ReadBuffer(bytesNested, f.FieldScheme)
		} else {
			res = ReadBuffer(b.tab.Bytes, f.FieldScheme)
			res.tab.Pos = b.tab.Indirect(uOffsetT)
		}
		if !f.IsArray {
			b.prepareModifiedFields()
			if b.modifiedFields[f.order] == nil {
				b.set(f, res)
			}
		}
		res.owner = b
		return res
	default:
		return byteSliceToString(b.tab.ByteVector(uOffsetT))
	}
}

func getFBFieldSize(ft FieldType) int {
	switch ft {
	case FieldTypeBool:
		return flatbuffers.SizeBool
	case FieldTypeByte:
		return flatbuffers.SizeByte
	case FieldTypeDouble:
		return flatbuffers.SizeFloat64
	case FieldTypeFloat:
		return flatbuffers.SizeFloat32
	case FieldTypeInt:
		return flatbuffers.SizeInt32
	case FieldTypeLong:
		return flatbuffers.SizeInt64
	default:
		return flatbuffers.SizeUOffsetT
	}
}

// Get returns stored field value by name.
// field is scalar -> scalar is returned
// field is an array of scalars -> []T is returned
// field is a nested object -> *dynobuffers.Buffer is returned
// field is an array of nested objects -> *dynobuffers.ObjectArray is returned
// field is not set, set to nil or no such field in the Scheme -> nil
func (b *Buffer) Get(name string) interface{} {
	f, ok := b.Scheme.FieldsMap[name]
	if !ok {
		return nil
	}
	return b.getByField(f, -1)
}

// GetByIndex returns array field element by its index
// no such field, index out of bounds, array field is not set or unset -> nil
func (b *Buffer) GetByIndex(name string, index int) interface{} {
	f, ok := b.Scheme.FieldsMap[name]
	if !ok || index < 0 {
		return nil
	}
	return b.getByField(f, index)
}

// ReadBuffer creates Buffer from bytes using provided Scheme
func ReadBuffer(bytes []byte, Scheme *Scheme) *Buffer {
	b := NewBuffer(Scheme)
	b.tab.Bytes = bytes
	b.tab.Pos = flatbuffers.GetUOffsetT(bytes)
	return b
}

// Set sets field value by name.
// Underlying byte array is not modified.
// Call ToBytes() to get modified byte array
func (b *Buffer) Set(name string, value interface{}) {
	f, ok := b.Scheme.FieldsMap[name]
	if !ok {
		return
	}
	b.set(f, value)
}

func (b *Buffer) setModified() {
	b.isModified = true
	if b.owner != nil {
		b.owner.setModified()
	}
}

func (b *Buffer) set(f *Field, value interface{}) {
	b.prepareModifiedFields()
	b.modifiedFields[f.order] = &modifiedField{value, false}
	if bNested, ok := value.(*Buffer); ok {
		bNested.owner = b
	}
	b.setModified()
}

// Append s.e.
func (b *Buffer) Append(name string, toAppend interface{}) {
	f, ok := b.Scheme.FieldsMap[name]
	if !ok {
		return
	}
	b.append(f, toAppend)
}

func (b *Buffer) append(f *Field, toAppend interface{}) {
	b.prepareModifiedFields()
	b.modifiedFields[f.order] = &modifiedField{toAppend, true}
	b.setModified()
}

// ApplyJSONAndToBytes sets field values described by provided json and returns new FlatBuffer byte array
// See `ApplyMap` for details
func (b *Buffer) ApplyJSONAndToBytes(jsonBytes []byte) ([]byte, error) {
	dest := map[string]interface{}{}
	err := json.Unmarshal(jsonBytes, &dest)
	if err != nil {
		return nil, err
	}
	err = b.ApplyMap(dest)
	if err != nil {
		return nil, err
	}

	return b.ToBytes()
}

// ApplyMap sets field values described by provided map[string]interface{}
// Resulting buffer has no value (or has nil value) for a mandatory field -> error
// Value type and field type are incompatible (e.g. string for numberic field) -> error
// Value and field types differs but value fits into field -> no error. Examples:
//   255 fits into float, double, int, long, byte;
//   256 does not fit into byte
//   math.MaxInt64 does not fit into int32
// Unexisting field is provided -> error
// Previousy stored or modified data is rewritten with the provided data
// Byte arrays are expected to be base64 strings
// Array element is nil -> error (not supported)
func (b *Buffer) ApplyMap(data map[string]interface{}) error {
	for fn, fv := range data {
		f, ok := b.Scheme.FieldsMap[fn]
		if !ok {
			return fmt.Errorf("field %s does not exist in the scheme", fn)
		}
		if fv == nil {
			b.set(f, nil)
			continue
		}
		if f.Ft == FieldTypeObject {
			if f.IsArray {
				datasNested, ok := fv.([]interface{})
				if !ok {
					return fmt.Errorf("array of objects required but %#v provided for field %s", fv, f.QualifiedName())
				}
				buffers := make([]*Buffer, len(datasNested))
				for i, dataNestedIntf := range datasNested {
					dataNested, ok := dataNestedIntf.(map[string]interface{})
					if !ok {
						return fmt.Errorf("element value of array field %s must be an object, %#v provided", fn, dataNestedIntf)
					}
					buffers[i] = NewBuffer(f.FieldScheme)
					buffers[i].owner = b
					buffers[i].ApplyMap(dataNested)
				}
				b.append(f, buffers)
			} else {
				bNested := NewBuffer(f.FieldScheme)
				bNested.owner = b
				dataNested, ok := fv.(map[string]interface{})
				if !ok {
					return fmt.Errorf("value of field %s must be an object, %#v provided", fn, fv)
				}
				bNested.ApplyMap(dataNested)
				b.set(f, bNested)
			}
		} else {
			if f.IsArray {
				if f.Ft == FieldTypeByte {
					base64Str, ok := fv.(string)
					if !ok {
						return fmt.Errorf("base64 encoded byte array is expected for %s, %#v provided", f.QualifiedName(), fv)
					}
					var err error
					fv, err = base64.StdEncoding.DecodeString(base64Str)
					if err != nil {
						return err
					}
				}
				b.append(f, fv)
			} else {
				b.set(f, fv)
			}
		}
	}
	return nil
}

// ToBytes returns new FlatBuffer byte array with fields modified by Set() and fields which initially had values
// Note: initial byte array and current modifications are kept
func (b *Buffer) ToBytes() ([]byte, error) {
	if !b.isModified && len(b.tab.Bytes) > 0 {
		return b.tab.Bytes, nil
	}
	bl := flatbuffers.NewBuilder(0)
	_, err := b.encodeBuffer(bl)
	if err != nil {
		return nil, err
	}
	return bl.FinishedBytes(), nil
}

func (b *Buffer) prepareModifiedFields() {
	if len(b.modifiedFields) == 0 {
		b.modifiedFields = make([]*modifiedField, len(b.Scheme.Fields))
	}
}

type offset struct {
	str flatbuffers.UOffsetT
	obj flatbuffers.UOffsetT
	arr flatbuffers.UOffsetT
}

func (b *Buffer) encodeBuffer(bl *flatbuffers.Builder) (flatbuffers.UOffsetT, error) {
	offsets := make([]offset, len(b.Scheme.Fields))
	b.prepareModifiedFields()
	var err error

	for _, f := range b.Scheme.Fields {
		if f.IsArray {
			arrayUOffsetT := flatbuffers.UOffsetT(0)
			modifiedField := b.modifiedFields[f.order]
			if modifiedField != nil {
				if modifiedField.value != nil && !reflect.ValueOf(modifiedField.value).IsNil() {
					var toAppendToIntf interface{} = nil
					if modifiedField.isAppend {
						toAppendToIntf = b.getByField(f, -1)
					}
					if arrayUOffsetT, err = b.encodeArray(bl, f, modifiedField.value, toAppendToIntf); err != nil {
						return 0, err
					}
				}
			} else {
				if uOffsetT := b.getFieldUOffsetTByOrder(f.order); uOffsetT != 0 {
					// copy from source bytes if not modified and initially existed
					if isFixedSizeField(f) {
						// copy fixed-size array as byte array
						arrayLen := b.tab.VectorLen(uOffsetT - b.tab.Pos)
						uOffsetT = b.tab.Vector(uOffsetT - b.tab.Pos)
						arrayUOffsetT = bl.CreateByteVector(b.tab.Bytes[uOffsetT : int(uOffsetT)+arrayLen])
					} else {
						// re-encode var-size array
						if existingArray := b.getByUOffsetT(f, -1, uOffsetT); existingArray != nil {
							arrayUOffsetT, _ = b.encodeArray(bl, f, existingArray, nil) // no errors should be here
						}
					}
				}
			}
			offsets[f.order].arr = arrayUOffsetT
		} else if f.Ft == FieldTypeObject {
			nestedUOffsetT := flatbuffers.UOffsetT(0)
			modifiedField := b.modifiedFields[f.order]
			if modifiedField != nil {
				if modifiedField.value != nil {
					if nestedBuffer, ok := modifiedField.value.(*Buffer); !ok {
						return 0, fmt.Errorf("nested object required but %#v provided for field %s", modifiedField.value, f.QualifiedName())
					} else if storeObjectsAsBytes {
						nestedBytes, err := nestedBuffer.ToBytes()
						if err != nil {
							return 0, fmt.Errorf("failed to encode nested object %s: %s", f.QualifiedName(), err)
						}
						nestedUOffsetT = bl.CreateByteVector(nestedBytes)
					} else if nestedUOffsetT, err = nestedBuffer.encodeBuffer(bl); err != nil {
						return 0, err
					}
				}
			} else {
				if uOffsetT := b.getFieldUOffsetTByOrder(f.order); uOffsetT != 0 {
					bufToWrite := b.getByUOffsetT(f, -1, uOffsetT) // can not be nil
					if storeObjectsAsBytes {
						nestedBytes, _ := bufToWrite.(*Buffer).ToBytes() // no errors should be here
						nestedUOffsetT = bl.CreateByteVector(nestedBytes)
					} else {
						nestedUOffsetT, _ = bufToWrite.(*Buffer).encodeBuffer(bl) // no errors should be here
					}
				}
			}
			offsets[f.order].obj = nestedUOffsetT
		} else if f.Ft == FieldTypeString {
			modifiedStringField := b.modifiedFields[f.order]
			if modifiedStringField != nil {
				if modifiedStringField.value != nil {
					if strToWrite, ok := modifiedStringField.value.(string); ok {
						offsets[f.order].str = bl.CreateString(strToWrite)
					} else {
						return 0, fmt.Errorf("string required but %#v provided for field %s", modifiedStringField.value, f.QualifiedName())
					}
				}
			} else {
				if strToWrite, ok := b.getByStringField(f); ok {
					offsets[f.order].str = bl.CreateString(strToWrite)
				}
			}
		}
	}

	bl.StartObject(len(b.Scheme.Fields))
	for _, f := range b.Scheme.Fields {
		isSet := false
		if f.IsArray {
			if isSet = offsets[f.order].arr > 0; isSet {
				bl.PrependUOffsetTSlot(f.order, offsets[f.order].arr, 0)
			}
		} else {
			switch f.Ft {
			case FieldTypeString:
				if isSet = offsets[f.order].str > 0; isSet {
					bl.PrependUOffsetTSlot(f.order, offsets[f.order].str, 0)
				}
			case FieldTypeObject:
				if isSet = offsets[f.order].obj > 0; isSet {
					bl.PrependUOffsetTSlot(f.order, offsets[f.order].obj, 0)
				}
			default:
				modifiedField := b.modifiedFields[f.order]
				if modifiedField != nil {
					if isSet = modifiedField.value != nil; isSet {
						if !encodeFixedSizeValue(bl, f, modifiedField.value) {
							return 0, fmt.Errorf("wrong value %T(%#v) provided for field %s", modifiedField.value, modifiedField.value, f.QualifiedName())
						}
					}
				} else {
					isSet = copyFixedSizeValue(bl, b, f)
				}
			}
		}
		if f.IsMandatory && !isSet {
			return 0, fmt.Errorf("Mandatory field %s is not set", f.QualifiedName())
		}
	}
	res := bl.EndObject()
	bl.Finish(res)
	return res, nil
}

// HasValue returns if specified field exists in the scheme and its value is set to non-nil
func (b *Buffer) HasValue(name string) bool {
	return b.getFieldUOffsetT(name) != 0
}

func intfToInt32Arr(f *Field, value interface{}) ([]int32, bool) {
	arr, ok := value.([]int32)
	if !ok {
		intfs, ok := value.([]interface{})
		if !ok {
			return nil, false
		}
		arr = make([]int32, len(intfs))
		for i, intf := range intfs {
			float64Src, ok := intf.(float64)
			if !ok || !isFloat64ValueFitsIntoField(f, float64Src) {
				return nil, false
			}
			arr[i] = int32(float64Src)
		}
	}
	return arr, true
}

func intfToBoolArr(f *Field, value interface{}) ([]bool, bool) {
	arr, ok := value.([]bool)
	if !ok {
		intfs, ok := value.([]interface{})
		if !ok {
			return nil, false
		}
		arr = make([]bool, len(intfs))
		for i, intf := range intfs {
			boolVal, ok := intf.(bool)
			if !ok {
				return nil, false
			}
			arr[i] = boolVal
		}
	}
	return arr, true
}

func intfToInt64Arr(f *Field, value interface{}) ([]int64, bool) {
	arr, ok := value.([]int64)
	if !ok {
		intfs, ok := value.([]interface{})
		if !ok {
			return nil, false
		}
		arr = make([]int64, len(intfs))
		for i, intf := range intfs {
			float64Src, ok := intf.(float64)
			if !ok || !isFloat64ValueFitsIntoField(f, float64Src) {
				return nil, false
			}
			arr[i] = int64(float64Src)
		}
	}
	return arr, true
}

func intfToFloat32Arr(f *Field, value interface{}) ([]float32, bool) {
	arr, ok := value.([]float32)
	if !ok {
		intfs, ok := value.([]interface{})
		if !ok {
			return nil, false
		}
		arr = make([]float32, len(intfs))
		for i, intf := range intfs {
			float64Src, ok := intf.(float64)
			if !ok || !isFloat64ValueFitsIntoField(f, float64Src) {
				return nil, false
			}
			arr[i] = float32(float64Src)
		}
	}
	return arr, true
}

func intfToFloat64Arr(f *Field, value interface{}) ([]float64, bool) {
	arr, ok := value.([]float64)
	if !ok {
		intfs, ok := value.([]interface{})
		if !ok {
			return nil, false
		}
		arr = make([]float64, len(intfs))
		for i, intf := range intfs {
			float64Src, ok := intf.(float64)
			if !ok {
				return nil, false
			}
			arr[i] = float64Src
		}
	}
	return arr, true
}

func (b *Buffer) encodeArray(bl *flatbuffers.Builder, f *Field, value interface{}, toAppendToIntf interface{}) (flatbuffers.UOffsetT, error) {
	elemSize := getFBFieldSize(f.Ft)
	switch f.Ft {
	case FieldTypeInt:
		arr, ok := intfToInt32Arr(f, value)
		if !ok {
			return 0, fmt.Errorf("[]int32 required but %#v provided for field %s", value, f.QualifiedName())
		}
		if toAppendToIntf != nil {
			toAppendTo := toAppendToIntf.([]int32)
			toAppendTo = append(toAppendTo, arr...)
			arr = toAppendTo
		}
		if len(arr) == 0 {
			return bl.CreateByteVector([]byte{}), nil
		}

		length := len(arr) * flatbuffers.SizeInt32
		hdr := reflect.SliceHeader{Data: uintptr(unsafe.Pointer(&arr[0])), Len: length, Cap: length}
		target := *(*[]byte)(unsafe.Pointer(&hdr))
		return bl.CreateByteVector(target), nil
	case FieldTypeBool:
		arr, ok := intfToBoolArr(f, value)
		if !ok {
			return 0, fmt.Errorf("[]bool required but %#v provided for field %s", value, f.QualifiedName())
		}
		if toAppendToIntf != nil {
			toAppendTo := toAppendToIntf.([]bool)
			toAppendTo = append(toAppendTo, arr...)
			arr = toAppendTo
		}
		if len(arr) == 0 {
			return bl.CreateByteVector([]byte{}), nil
		}
		length := len(arr) * flatbuffers.SizeBool
		hdr := reflect.SliceHeader{Data: uintptr(unsafe.Pointer(&arr[0])), Len: length, Cap: length}
		target := *(*[]byte)(unsafe.Pointer(&hdr))
		return bl.CreateByteVector(target), nil
	case FieldTypeLong:
		arr, ok := intfToInt64Arr(f, value)
		if !ok {
			return 0, fmt.Errorf("[]int64 required but %#v provided for field %s", value, f.QualifiedName())
		}
		if toAppendToIntf != nil {
			toAppendTo := toAppendToIntf.([]int64)
			toAppendTo = append(toAppendTo, arr...)
			arr = toAppendTo
		}
		if len(arr) == 0 {
			return bl.CreateByteVector([]byte{}), nil
		}
		length := len(arr) * flatbuffers.SizeInt64
		hdr := reflect.SliceHeader{Data: uintptr(unsafe.Pointer(&arr[0])), Len: length, Cap: length}
		target := *(*[]byte)(unsafe.Pointer(&hdr))
		return bl.CreateByteVector(target), nil
	case FieldTypeFloat:
		arr, ok := intfToFloat32Arr(f, value)
		if !ok {
			return 0, fmt.Errorf("[]float32 required but %#v provided for field %s", value, f.QualifiedName())
		}
		if toAppendToIntf != nil {
			toAppendTo := toAppendToIntf.([]float32)
			toAppendTo = append(toAppendTo, arr...)
			arr = toAppendTo
		}
		if len(arr) == 0 {
			return bl.CreateByteVector([]byte{}), nil
		}
		length := len(arr) * flatbuffers.SizeFloat32
		hdr := reflect.SliceHeader{Data: uintptr(unsafe.Pointer(&arr[0])), Len: length, Cap: length}
		target := *(*[]byte)(unsafe.Pointer(&hdr))
		return bl.CreateByteVector(target), nil
	case FieldTypeDouble:
		arr, ok := intfToFloat64Arr(f, value)
		if !ok {
			return 0, fmt.Errorf("[]float32 required but %#v provided for field %s", value, f.QualifiedName())
		}
		if toAppendToIntf != nil {
			toAppendTo := toAppendToIntf.([]float64)
			toAppendTo = append(toAppendTo, arr...)
			arr = toAppendTo
		}
		if len(arr) == 0 {
			return bl.CreateByteVector([]byte{}), nil
		}
		length := len(arr) * flatbuffers.SizeFloat64
		hdr := reflect.SliceHeader{Data: uintptr(unsafe.Pointer(&arr[0])), Len: length, Cap: length}
		target := *(*[]byte)(unsafe.Pointer(&hdr))
		return bl.CreateByteVector(target), nil
	case FieldTypeByte:
		arr, ok := value.([]byte)
		if toAppendToIntf != nil {
			toAppendTo := toAppendToIntf.([]byte)
			toAppendTo = append(toAppendTo, arr...)
			arr = toAppendTo
		}
		if !ok {
			return 0, fmt.Errorf("[]byte required but %#v provided for field %s", value, f.QualifiedName())
		}
		return bl.CreateByteVector(arr), nil
	case FieldTypeString:
		var arr []string
		switch value.(type) {
		case []string:
			// Set("", []string) was called
			arr = value.([]string)
		case []interface{}:
			// came from JSON
			intfs := value.([]interface{})
			arr = make([]string, len(intfs))
			for i, intf := range intfs {
				stringVal, ok := intf.(string)
				if !ok {
					return 0, fmt.Errorf("[]byte required but %#v provided for field %s", value, f.QualifiedName())
				}
				arr[i] = stringVal
			}
		default:
			return 0, fmt.Errorf("%#v provided for field %s which can not be converted to []string", value, f.QualifiedName())
		}
		if toAppendToIntf != nil {
			toAppendTo := toAppendToIntf.([]string)
			toAppendTo = append(toAppendTo, arr...)
			arr = toAppendTo
		}
		stringUOffsetTs := make([]flatbuffers.UOffsetT, len(arr))
		for i := 0; i < len(arr); i++ {
			stringUOffsetTs[i] = bl.CreateString(arr[i])
		}
		bl.StartVector(elemSize, len(arr), elemSize)
		for i := len(arr) - 1; i >= 0; i-- {
			bl.PrependUOffsetT(stringUOffsetTs[i])
		}
		return bl.EndVector(len(arr)), nil
	default:
		nestedUOffsetTs := []flatbuffers.UOffsetT{}
		switch value.(type) {
		case []*Buffer:
			// explicit Set\Append("", []*Buffer) was called
			arr := value.([]*Buffer)
			for i := 0; i < len(arr); i++ {
				if arr[i] == nil {
					return 0, fmt.Errorf("nil element of array field %s is provided. Nils are not supported for array elements", f.QualifiedName())
				}
				if storeObjectsAsBytes {
					nestedBytes, err := arr[i].ToBytes()
					if err != nil {
						return 0, err
					}
					nestedUOffsetTs = append(nestedUOffsetTs, bl.CreateByteVector(nestedBytes))
				} else {
					nestedUOffsetT, err := arr[i].encodeBuffer(bl)
					if err != nil {
						return 0, err
					}
					nestedUOffsetTs = append(nestedUOffsetTs, nestedUOffsetT)
				}
			}
		case *ObjectArray:
			arr := value.(*ObjectArray)
			for arr.Next() {
				if storeObjectsAsBytes {
					nestedBytes, _ := arr.Buffer.ToBytes()
					nestedUOffsetTs = append(nestedUOffsetTs, bl.CreateByteVector(nestedBytes))
				} else {
					nestedUOffsetT, _ := arr.Buffer.encodeBuffer(bl) // should be no errors here
					nestedUOffsetTs = append(nestedUOffsetTs, nestedUOffsetT)
				}
			}

		default:
			return 0, fmt.Errorf("%#v provided for field %s is not an array of nested objects", value, f.QualifiedName())
		}

		if toAppendToIntf != nil {
			toAppendToArr := toAppendToIntf.(*ObjectArray)
			toAppendToUOffsetTs := make([]flatbuffers.UOffsetT, toAppendToArr.Len)
			for i := 0; toAppendToArr.Next(); i++ {
				if storeObjectsAsBytes {
					bufBytes, _ := toAppendToArr.Buffer.ToBytes()
					toAppendToUOffsetTs[i] = bl.CreateByteVector(bufBytes)
				} else {
					toAppendToUOffsetTs[i], _ = toAppendToArr.Buffer.encodeBuffer(bl)
				}
			}
			toAppendToUOffsetTs = append(toAppendToUOffsetTs, nestedUOffsetTs...)
			nestedUOffsetTs = toAppendToUOffsetTs
		}

		bl.StartVector(elemSize, len(nestedUOffsetTs), elemSize)
		for i := len(nestedUOffsetTs) - 1; i >= 0; i-- {
			bl.PrependUOffsetT(nestedUOffsetTs[i])
		}
		return bl.EndVector(len(nestedUOffsetTs)), nil

	}
}

func copyFixedSizeValue(dest *flatbuffers.Builder, src *Buffer, f *Field) bool {
	offset := src.getFieldUOffsetTByOrder(f.order)
	if offset == 0 {
		return false
	}
	switch f.Ft {
	case FieldTypeInt:
		dest.PrependInt32(src.tab.GetInt32(offset))
	case FieldTypeLong:
		dest.PrependInt64(src.tab.GetInt64(offset))
	case FieldTypeFloat:
		dest.PrependFloat32(src.tab.GetFloat32(offset))
	case FieldTypeDouble:
		dest.PrependFloat64(src.tab.GetFloat64(offset))
	case FieldTypeByte:
		dest.PrependByte(src.tab.GetByte(offset))
	case FieldTypeBool:
		dest.PrependBool(src.tab.GetBool(offset))
	}
	dest.Slot(f.order)
	return true
}

func isFloat64ValueFitsIntoField(f *Field, float64Src float64) bool {
	if float64Src == 0 {
		return true
	}
	if float64Src == float64(int32(float64Src)) {
		if float64Src >= 0 && float64Src <= 255 {
			return f.Ft == FieldTypeInt || f.Ft == FieldTypeLong || f.Ft == FieldTypeDouble || f.Ft == FieldTypeFloat || f.Ft == FieldTypeByte
		}
		return f.Ft == FieldTypeInt || f.Ft == FieldTypeLong || f.Ft == FieldTypeDouble || f.Ft == FieldTypeFloat
	} else if float64Src == float64(int64(float64Src)) {
		return f.Ft == FieldTypeLong || f.Ft == FieldTypeDouble
	} else {
		return f.Ft == FieldTypeDouble || f.Ft == FieldTypeFloat
	}
}

func encodeFixedSizeValue(bl *flatbuffers.Builder, f *Field, value interface{}) bool {
	switch res := value.(type) {
	case bool:
		if f.Ft != FieldTypeBool {
			return false
		}
		bl.PrependBool(res)
	case float64:
		if !isFloat64ValueFitsIntoField(f, res) {
			return false
		}
		switch f.Ft {
		case FieldTypeInt:
			bl.PrependInt32(int32(res))
		case FieldTypeLong:
			bl.PrependInt64(int64(res))
		case FieldTypeFloat:
			bl.PrependFloat32(float32(res))
		case FieldTypeDouble:
			bl.PrependFloat64(res)
		default:
			bl.PrependByte(byte(res))
		}
	case float32:
		if f.Ft != FieldTypeFloat {
			return false
		}
		bl.PrependFloat32(res)
	case int64:
		if f.Ft != FieldTypeLong {
			return false
		}
		bl.PrependInt64(res)
	case int32:
		if f.Ft != FieldTypeInt {
			return false
		}
		bl.PrependInt32(res)
	case byte:
		if f.Ft != FieldTypeByte {
			return false
		}
		bl.PrependByte(res)
	case int:
		switch f.Ft {
		case FieldTypeInt:
			if math.Abs(float64(res)) > math.MaxInt32 {
				return false
			}
			bl.PrependInt32(int32(res))
		case FieldTypeLong:
			if math.Abs(float64(res)) > math.MaxInt64 {
				return false
			}
			bl.PrependInt64(int64(res))
		default:
			if math.Abs(float64(res)) > 255 {
				return false
			}
			bl.PrependByte(byte(res))
		}
	default:
		return false
	}
	bl.Slot(f.order)
	return true
}

// ToJSON returns JSON key->value string
func (b *Buffer) ToJSON() []byte {
	buf := bytes.NewBufferString("")
	e := json.NewEncoder(buf)
	buf.WriteString("{")
	for _, f := range b.Scheme.Fields {
		var value interface{}
		if len(b.modifiedFields) == 0 {
			value = b.getByField(f, -1)
		} else {
			modifiedField := b.modifiedFields[f.order]
			if modifiedField != nil {
				value = modifiedField.value
			} else {
				value = b.getByField(f, -1)
			}
		}
		if value != nil {
			buf.WriteString("\"" + f.Name + "\":")
			if f.Ft == FieldTypeObject {
				if f.IsArray {
					buf.WriteString("[")
					if arr, ok := value.(*ObjectArray); ok {
						for arr.Next() {
							buf.Write(arr.Buffer.ToJSON())
							buf.WriteString(",")
						}
					} else {
						buffers, _ := value.([]*Buffer)
						for _, buffer := range buffers {
							buf.Write(buffer.ToJSON())
							buf.WriteString(",")
						}
					}
					buf.Truncate(buf.Len() - 1)
					buf.WriteString("]")
				} else {
					buf.Write(value.(*Buffer).ToJSON())
				}
			} else {
				e.Encode(value)
			}
			buf.WriteString(",")
		}
	}
	if buf.Len() > 1 {
		buf.Truncate(buf.Len() - 1)
	}
	buf.WriteString("}")
	return []byte(strings.Replace(buf.String(), "\n", "", -1))
}

// GetBytes returns underlying byte buffer
func (b *Buffer) GetBytes() []byte {
	return b.tab.Bytes
}

// GetNames returns list of field names which values are non-nil in current buffer
// Set() fields are not considered
// fields of nested objects are not considered
func (b *Buffer) GetNames() []string {
	res := []string{}
	if len(b.tab.Bytes) == 0 {
		return res
	}

	vtable := flatbuffers.UOffsetT(flatbuffers.SOffsetT(b.tab.Pos) - b.tab.GetSOffsetT(b.tab.Pos))
	vOffsetT := b.tab.GetVOffsetT(vtable)

	for order := 0; true; order++ {
		vTableOffset := flatbuffers.VOffsetT((order + 2) * 2)
		if vTableOffset >= vOffsetT {
			break
		}
		if b.tab.GetVOffsetT(vtable+flatbuffers.UOffsetT(vTableOffset)) > 0 {
			res = append(res, b.Scheme.Fields[order].Name)
		}
	}
	return res
}

// NewScheme creates new empty Scheme
func NewScheme() *Scheme {
	return &Scheme{"", map[string]*Field{}, []*Field{}}
}

// AddField adds field
func (s *Scheme) AddField(name string, ft FieldType, isMandatory bool) {
	s.AddFieldC(name, ft, nil, isMandatory, false)
}

// AddArray adds array field
func (s *Scheme) AddArray(name string, elementType FieldType, isMandatory bool) {
	s.AddFieldC(name, elementType, nil, isMandatory, true)
}

// AddNested adds nested object field
func (s *Scheme) AddNested(name string, nested *Scheme, isMandatory bool) {
	s.AddFieldC(name, FieldTypeObject, nested, isMandatory, false)
}

// AddNestedArray adds array of nested objects field
func (s *Scheme) AddNestedArray(name string, nested *Scheme, isMandatory bool) {
	s.AddFieldC(name, FieldTypeObject, nested, isMandatory, true)
}

// AddFieldC adds new finely-tuned field
func (s *Scheme) AddFieldC(name string, ft FieldType, nested *Scheme, isMandatory bool, IsArray bool) {
	newField := &Field{name, ft, len(s.FieldsMap), isMandatory, nested, s, IsArray}
	s.FieldsMap[name] = newField
	s.Fields = append(s.Fields, newField)
}

// MarshalYAML marshals Scheme to yaml. Needs to conform to yaml.Marshaler interface
func (s *Scheme) MarshalYAML() (interface{}, error) {
	res := yaml.MapSlice{}
	for _, f := range s.Fields {
		for ftStr, curFt := range yamlFieldTypesMap {
			if curFt == f.Ft {
				fieldName := f.Name
				if f.IsMandatory {
					fnBytes := []byte(fieldName)
					fnBytes[0] = []byte(strings.ToUpper(fieldName))[0]
					fieldName = string(fnBytes)
				}
				if f.IsArray {
					fieldName = fieldName + ".."
				}
				var val interface{}
				if f.Ft == FieldTypeObject {
					valTemp, err := f.FieldScheme.MarshalYAML()
					if err != nil {
						return nil, err
					}
					val = valTemp
				} else {
					val = ftStr
				}
				item := yaml.MapItem{Key: fieldName, Value: val}
				res = append(res, item)
			}
		}
	}
	return res, nil
}

// UnmarshalYAML unmarshals Scheme from yaml. Needs to conform to yaml.Unmarshaler interface
func (s *Scheme) UnmarshalYAML(unmarshal func(interface{}) error) error {
	mapSlice := yaml.MapSlice{}
	if err := unmarshal(&mapSlice); err != nil {
		return err
	}
	newS, err := MapSliceToScheme(mapSlice)
	if err != nil {
		return err
	}
	s.Fields = newS.Fields
	s.FieldsMap = newS.FieldsMap
	return nil
}

// GetNestedScheme returns Scheme of nested object if the field has FieldTypeObject type, nil otherwise
func (s *Scheme) GetNestedScheme(nestedObjectField string) *Scheme {
	if f, ok := s.FieldsMap[nestedObjectField]; ok {
		return f.FieldScheme
	}
	return nil
}

// QualifiedName returns ownerScheme.fieldName
func (f *Field) QualifiedName() string {
	if len(f.ownerScheme.Name) > 0 {
		return f.ownerScheme.Name + "." + f.Name
	}
	return f.Name
}

// YamlToScheme creates Scheme by provided yaml `fieldName: yamlFieldType`
// Field types:
//   - `int` -> `int32`
//   - `long` -> `int64`
//   - `float` -> `float32`
//   - `double` -> `float64`
//   - `bool` -> `bool`
//   - `string` -> `string`
//   - `byte` -> `byte`
// Field name starts with the capital letter -> field is mandatory
// Field name ends with `..` -> field is an array
// See [dynobuffers_test.go](dynobuffers_test.go) for examples
func YamlToScheme(yamlStr string) (*Scheme, error) {
	mapSlice := yaml.MapSlice{}
	err := yaml.Unmarshal([]byte(yamlStr), &mapSlice)
	if err != nil {
		return nil, err
	}
	return MapSliceToScheme(mapSlice)
}

// MapSliceToScheme s.e.
func MapSliceToScheme(mapSlice yaml.MapSlice) (*Scheme, error) {
	res := NewScheme()
	for _, mapItem := range mapSlice {
		if nestedMapSlice, ok := mapItem.Value.(yaml.MapSlice); ok {
			fieldName, isMandatory, IsArray := fieldPropsFromYaml(mapItem.Key.(string))
			nestedScheme, err := MapSliceToScheme(nestedMapSlice)
			nestedScheme.Name = fieldName
			if err != nil {
				return nil, err
			}
			if IsArray {
				res.AddNestedArray(fieldName, nestedScheme, isMandatory)
			} else {
				res.AddNested(fieldName, nestedScheme, isMandatory)
			}
		} else if typeStr, ok := mapItem.Value.(string); ok {
			fieldName, isMandatory, IsArray := fieldPropsFromYaml(mapItem.Key.(string))
			if ft, ok := yamlFieldTypesMap[typeStr]; ok {
				if IsArray {
					res.AddArray(fieldName, ft, isMandatory)
				} else {
					res.AddField(fieldName, ft, isMandatory)
				}
			} else {
				return nil, errors.New("unknown field type: " + typeStr)
			}
		}
	}

	return res, nil
}

// SchemeFromStruct creates new Scheme by provided struct.
// Mandatory = is pointer field
// not a struct or not pointer to struct -> error
// unsupported type -> error
func SchemeFromStruct(strct interface{}) (s *Scheme, err error) {
	strctValue := reflect.ValueOf(strct)
	strctType := reflect.TypeOf(strct)
	if strctType.Kind() == reflect.Ptr {
		strctType = strctType.Elem()
		strctValue = strctValue.Elem()
	}
	if strctType.Kind() != reflect.Struct {
		return nil, fmt.Errorf("struct expected, %v provided", strct)
	}

	return processStrcutType(strctType, strctValue)
}

func processStrcutType(strctType reflect.Type, strctValue reflect.Value) (s *Scheme, err error) {
	s = NewScheme()
	for i := 0; i < strctType.NumField(); i++ {
		var ft FieldType
		f := strctType.Field(i)

		ft, isArray := getFieldDesc(f, f.Type)
		if ft == FieldTypeUnspecified {
			return nil, fmt.Errorf("unsupported field type: %s %s", f.Name, f.Type.Kind())
		}
		isMandatory := strctValue.Field(i).CanInterface()
		var nested *Scheme
		fieldName := f.Name
		if isMandatory {
			fnBytes := []byte(fieldName)
			fnBytes[0] = []byte(strings.ToLower(string(fnBytes[0])))[0]
			fieldName = string(fnBytes)
		}
		if ft == FieldTypeObject {
			nestedType := f.Type
			nestedValue := strctValue.Field(i)
			if nestedType.Kind() == reflect.Ptr {
				nestedType = nestedType.Elem()
				if isArray {
					nestedType = nestedType.Elem()
				}
				if nestedValue.IsZero() {
					nestedValue = reflect.New(nestedType).Elem()
				} else {
					nestedValue = nestedValue.Elem()
				}
			}
			if isArray {
				nestedType = nestedType.Elem()
				if nestedType.Kind() == reflect.Ptr {
					nestedType = nestedType.Elem()
				}
				if nestedValue.IsZero() {
					nestedValue = reflect.New(nestedType).Elem()
				} else {
					nestedValue = nestedValue.Elem()
				}
			}
			nested, err = processStrcutType(nestedType, nestedValue)
			nested.Name = fieldName
			if err != nil {
				return nil, err
			}
		}
		
		s.AddFieldC(fieldName, ft, nested, isMandatory, isArray)
	}
	return s, nil
}

func getFieldDesc(f reflect.StructField, fieldType reflect.Type) (ft FieldType, isArray bool) {
	if fieldType.Kind() == reflect.Slice || fieldType.Kind() == reflect.Array {
		isArray = true
		fieldType = fieldType.Elem()
	}
	if fieldType.Kind() == reflect.Ptr {
		fieldType = fieldType.Elem()
	}
	ft = typeToFT(fieldType)
	return
}

func typeToFT(t reflect.Type) (ft FieldType) {
	switch t.Kind() {
	case reflect.Int32:
		ft = FieldTypeInt
	case reflect.Int64:
		ft = FieldTypeLong
	case reflect.Float32:
		ft = FieldTypeFloat
	case reflect.Float64:
		ft = FieldTypeDouble
	case reflect.Bool:
		ft = FieldTypeBool
	case reflect.String:
		ft = FieldTypeString
	case reflect.Uint8:
		ft = FieldTypeByte
	case reflect.Struct:
		ft = FieldTypeObject
	default:
		ft = FieldTypeUnspecified
	}
	return
}

func fieldPropsFromYaml(yamlStr string) (fieldName string, isMandatory bool, isArray bool) {
	isMandatory = unicode.IsUpper(rune(yamlStr[0]))

	isArray = strings.HasSuffix(yamlStr, "..")
	if isArray {
		yamlStr = yamlStr[:len(yamlStr)-2]
	}
	fieldName = yamlStr
	if isMandatory {
		fnBytes := []byte(fieldName)
		fnBytes[0] = []byte(strings.ToLower(string(fnBytes[0])))[0]
		fieldName = string(fnBytes)
	}
	return
}

// byteSliceToString converts a []byte to string without a heap allocation.
func byteSliceToString(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

func isFixedSizeField(f *Field) bool {
	return f.Ft != FieldTypeObject && f.Ft != FieldTypeString
}
