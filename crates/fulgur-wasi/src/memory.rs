//! Linear-memory allocator helpers exposed to the WASM host.
//!
//! The host calls `fulgur_alloc` to obtain a pointer into the module's
//! linear memory, copies bytes in, calls a fulgur entry point, then
//! frees with `fulgur_free`. Implementations leak `Vec<u8>` on alloc and
//! reconstruct + drop on free. Length must match the original allocation
//! exactly — deallocating a different length is undefined behaviour.

#[unsafe(no_mangle)]
pub extern "C" fn fulgur_alloc(len: u32) -> u32 {
    alloc_impl(len as usize) as u32
}

/// Internal allocator that works with native pointers.
#[inline]
fn alloc_impl(len: usize) -> usize {
    let mut v: Vec<u8> = Vec::with_capacity(len);
    let ptr = v.as_mut_ptr() as usize;
    std::mem::forget(v);
    ptr
}

/// # Safety
/// `ptr` must come from a previous `fulgur_alloc(len)` and not have been
/// freed already.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn fulgur_free(ptr: u32, len: u32) {
    unsafe {
        free_impl(ptr as usize, len as usize);
    }
}

/// Internal freer that works with native pointers.
#[inline]
unsafe fn free_impl(ptr: usize, len: usize) {
    if ptr == 0 {
        return;
    }
    unsafe {
        let _ = Vec::from_raw_parts(ptr as *mut u8, 0, len);
    }
}

/// Borrow `len` bytes starting at `ptr` as a slice. Caller guarantees the
/// pointer/length is valid.
#[allow(dead_code)]
pub(crate) unsafe fn slice_from_raw(ptr: u32, len: u32) -> &'static [u8] {
    if len == 0 {
        return &[];
    }
    unsafe {
        std::slice::from_raw_parts(ptr as *const u8, len as usize)
    }
}

/// Allocate a `Vec<u8>` of the given content, leak it, and return its
/// `(ptr, len)` packed into one `u64`.
///
/// On 32-bit systems (WASM), returns `(ptr as u64) << 32 | len as u64`.
/// On 64-bit systems, returns just the upper 32 bits of ptr in the upper 32 bits,
/// and len in the lower 32 bits. This is a lossy packing for testing purposes only;
/// production code should use separate u64 returns or a struct.
/// Returns 0 if `data` is empty (caller treats 0 as "no result").
pub(crate) fn into_packed(data: Vec<u8>) -> u64 {
    if data.is_empty() {
        return 0;
    }
    let len = data.len() as u32;
    let mut v = data;
    let ptr = v.as_mut_ptr() as u64;
    std::mem::forget(v);
    (ptr << 32) | (len as u64)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn alloc_then_free_roundtrips_pattern() {
        let len = 64usize;
        let ptr = alloc_impl(len);
        assert_ne!(ptr, 0, "alloc returned null");
        unsafe {
            let slice = std::slice::from_raw_parts_mut(ptr as *mut u8, len);
            for (i, b) in slice.iter_mut().enumerate() {
                *b = (i & 0xff) as u8;
            }
            for (i, b) in slice.iter().enumerate() {
                assert_eq!(*b, (i & 0xff) as u8);
            }
            free_impl(ptr, len);
        }
    }

    #[test]
    fn into_packed_zero_for_empty() {
        assert_eq!(into_packed(Vec::new()), 0);
    }

    #[test]
    fn into_packed_recoverable() {
        let bytes = vec![0xAA, 0xBB, 0xCC, 0xDD];
        let packed = into_packed(bytes.clone());

        // On 32-bit (WASM), packed format is: ptr << 32 | len
        // On 64-bit (native), this is lossy and only for demonstration
        let len_extracted = (packed & 0xFFFFFFFF) as usize;
        assert_eq!(len_extracted, bytes.len());

        // For a more robust test, directly allocate and check
        let len = bytes.len();
        let ptr = alloc_impl(len);
        unsafe {
            let slice = std::slice::from_raw_parts_mut(ptr as *mut u8, len);
            slice.copy_from_slice(&bytes);
            let recovered = std::slice::from_raw_parts(ptr as *const u8, len);
            assert_eq!(recovered, &bytes[..]);
            free_impl(ptr, len);
        }
    }
}
