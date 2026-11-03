//go:build darwin && cgo

package main

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation
#include <stdlib.h>
#include <string.h>
#include <Security/Security.h>

// SecItem defaults to the file-based keychain on macOS. Keeping that default
// makes existing SecKeychain generic-password entries discoverable.
static CFMutableDictionaryRef rw_query(const char *service, const char *account) {
	CFMutableDictionaryRef query = CFDictionaryCreateMutable(NULL, 0, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	if (!query) return NULL;
	CFStringRef serviceString = CFStringCreateWithCString(NULL, service, kCFStringEncodingUTF8);
	CFStringRef accountString = CFStringCreateWithCString(NULL, account, kCFStringEncodingUTF8);
	if (!serviceString || !accountString) {
		if (serviceString) CFRelease(serviceString);
		if (accountString) CFRelease(accountString);
		CFRelease(query);
		return NULL;
	}
	CFDictionarySetValue(query, kSecClass, kSecClassGenericPassword);
	CFDictionarySetValue(query, kSecAttrService, serviceString);
	CFDictionarySetValue(query, kSecAttrAccount, accountString);
	CFDictionarySetValue(query, kSecMatchLimit, kSecMatchLimitOne);
	CFRelease(serviceString);
	CFRelease(accountString);
	return query;
}

static OSStatus rw_load(const char *service, const char *account, void **out, CFIndex *size) {
	CFMutableDictionaryRef query = rw_query(service, account);
	if (!query) return errSecAllocate;
	CFDictionarySetValue(query, kSecReturnData, kCFBooleanTrue);
	CFTypeRef result = NULL;
	OSStatus status = SecItemCopyMatching(query, &result);
	CFRelease(query);
	if (status != errSecSuccess) return status;
	if (!result || CFGetTypeID(result) != CFDataGetTypeID()) {
		if (result) CFRelease(result);
		return errSecDecode;
	}
	CFDataRef data = (CFDataRef)result;
	*size = CFDataGetLength(data);
	*out = malloc((size_t)*size + 1);
	if (!*out) { CFRelease(result); return errSecAllocate; }
	if (*size > 0) memcpy(*out, CFDataGetBytePtr(data), (size_t)*size);
	CFRelease(result);
	return errSecSuccess;
}

static OSStatus rw_save(const char *service, const char *account, const void *bytes, CFIndex length) {
	CFMutableDictionaryRef query = rw_query(service, account);
	if (!query) return errSecAllocate;
	CFDataRef value = CFDataCreate(NULL, (const UInt8 *)bytes, length);
	if (!value) { CFRelease(query); return errSecAllocate; }
	CFMutableDictionaryRef changes = CFDictionaryCreateMutable(NULL, 0, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	if (!changes) { CFRelease(value); CFRelease(query); return errSecAllocate; }
	CFDictionarySetValue(changes, kSecValueData, value);
	OSStatus status = SecItemUpdate(query, changes);
	if (status == errSecItemNotFound) {
		CFMutableDictionaryRef add = CFDictionaryCreateMutableCopy(NULL, 0, query);
		if (!add) status = errSecAllocate;
		else {
			CFDictionaryRemoveValue(add, kSecMatchLimit);
			CFDictionarySetValue(add, kSecValueData, value);
			status = SecItemAdd(add, NULL);
			CFRelease(add);
			if (status == errSecDuplicateItem) status = SecItemUpdate(query, changes);
		}
	}
	CFRelease(changes);
	CFRelease(value);
	CFRelease(query);
	return status;
}

static OSStatus rw_delete(const char *service, const char *account) {
	CFMutableDictionaryRef query = rw_query(service, account);
	if (!query) return errSecAllocate;
	OSStatus status = SecItemDelete(query);
	CFRelease(query);
	return status;
}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

const keychainService = "RepoWatch"

func keychainError(status C.OSStatus) error {
	return fmt.Errorf("钥匙串错误 (OSStatus %d)", int(status))
}

func loadCredential(req RepoRequest) (string, error) {
	service := C.CString(keychainService)
	defer C.free(unsafe.Pointer(service))
	account := C.CString(credentialName(req))
	defer C.free(unsafe.Pointer(account))
	var data unsafe.Pointer
	var size C.CFIndex
	status := C.rw_load(service, account, &data, &size)
	if status == C.errSecItemNotFound {
		return "", nil
	}
	if status != C.errSecSuccess {
		return "", keychainError(status)
	}
	defer C.free(data)
	if size < 0 || size > 1<<20 {
		return "", fmt.Errorf("钥匙串中的令牌过长")
	}
	return string(C.GoBytes(data, C.int(size))), nil
}

func saveCredential(req RepoRequest, token string) error {
	service := C.CString(keychainService)
	defer C.free(unsafe.Pointer(service))
	account := C.CString(credentialName(req))
	defer C.free(unsafe.Pointer(account))
	data := C.CBytes([]byte(token))
	defer C.free(data)
	status := C.rw_save(service, account, data, C.CFIndex(len(token)))
	if status != C.errSecSuccess {
		return keychainError(status)
	}
	return nil
}

func deleteCredential(req RepoRequest) error {
	service := C.CString(keychainService)
	defer C.free(unsafe.Pointer(service))
	account := C.CString(credentialName(req))
	defer C.free(unsafe.Pointer(account))
	status := C.rw_delete(service, account)
	if status == C.errSecItemNotFound {
		return nil
	}
	if status != C.errSecSuccess {
		return keychainError(status)
	}
	return nil
}
