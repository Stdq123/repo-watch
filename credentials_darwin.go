//go:build darwin && cgo

package main

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation
#include <stdlib.h>
#include <Security/Security.h>
*/
import "C"

import (
	"fmt"
	"unsafe"
)

const keychainService = "RepoWatch"

func keychainFind(req RepoRequest) (C.SecKeychainItemRef, unsafe.Pointer, C.UInt32, C.OSStatus) {
	service := C.CString(keychainService)
	defer C.free(unsafe.Pointer(service))
	accountText := credentialName(req)
	account := C.CString(accountText)
	defer C.free(unsafe.Pointer(account))
	var item C.SecKeychainItemRef
	var data unsafe.Pointer
	var size C.UInt32
	status := C.SecKeychainFindGenericPassword(nil, C.UInt32(len(keychainService)), service, C.UInt32(len(accountText)), account, &size, &data, &item)
	return item, data, size, status
}
func keychainError(status C.OSStatus) error {
	return fmt.Errorf("钥匙串错误 (OSStatus %d)", int(status))
}
func loadCredential(req RepoRequest) (string, error) {
	item, data, size, status := keychainFind(req)
	if status == C.errSecItemNotFound {
		return "", nil
	}
	if status != C.errSecSuccess {
		return "", keychainError(status)
	}
	defer C.CFRelease(C.CFTypeRef(item))
	defer C.SecKeychainItemFreeContent(nil, data)
	return C.GoStringN((*C.char)(data), C.int(size)), nil
}
func saveCredential(req RepoRequest, token string) error {
	item, data, _, status := keychainFind(req)
	if status == C.errSecSuccess {
		defer C.CFRelease(C.CFTypeRef(item))
		defer C.SecKeychainItemFreeContent(nil, data)
		password := C.CBytes([]byte(token))
		defer C.free(password)
		status = C.SecKeychainItemModifyAttributesAndData(item, nil, C.UInt32(len(token)), password)
	} else if status == C.errSecItemNotFound {
		service := C.CString(keychainService)
		defer C.free(unsafe.Pointer(service))
		accountText := credentialName(req)
		account := C.CString(accountText)
		defer C.free(unsafe.Pointer(account))
		password := C.CBytes([]byte(token))
		defer C.free(password)
		status = C.SecKeychainAddGenericPassword(nil, C.UInt32(len(keychainService)), service, C.UInt32(len(accountText)), account, C.UInt32(len(token)), password, nil)
	}
	if status != C.errSecSuccess {
		return keychainError(status)
	}
	return nil
}
func deleteCredential(req RepoRequest) error {
	item, data, _, status := keychainFind(req)
	if status == C.errSecItemNotFound {
		return nil
	}
	if status != C.errSecSuccess {
		return keychainError(status)
	}
	defer C.CFRelease(C.CFTypeRef(item))
	defer C.SecKeychainItemFreeContent(nil, data)
	status = C.SecKeychainItemDelete(item)
	if status != C.errSecSuccess {
		return keychainError(status)
	}
	return nil
}
