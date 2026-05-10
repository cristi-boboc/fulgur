//! Thread-local last-error storage.
//!
//! WASI Preview 1 modules are single-threaded per instance, but the
//! storage is still `thread_local!` so the same code compiles + tests on
//! native multi-threaded targets.

use std::cell::RefCell;

thread_local! {
    static LAST_ERR: RefCell<String> = const { RefCell::new(String::new()) };
}

pub(crate) fn set(msg: impl Into<String>) {
    LAST_ERR.with(|e| *e.borrow_mut() = msg.into());
}

pub(crate) fn clear() {
    LAST_ERR.with(|e| e.borrow_mut().clear());
}

pub(crate) fn with<R>(f: impl FnOnce(&str) -> R) -> R {
    LAST_ERR.with(|e| f(&e.borrow()))
}

/// Length of the current last-error message in bytes.
#[unsafe(no_mangle)]
pub extern "C" fn fulgur_last_error_len() -> u32 {
    with(|s| s.len() as u32)
}

/// Copy up to `dst_len` bytes of the last error into `dst_ptr`. Returns
/// the number of bytes actually copied.
///
/// # Safety
/// `dst_ptr` must point to at least `dst_len` writable bytes inside WASM
/// linear memory. On native targets use `copy_impl` for testing instead.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn fulgur_last_error_copy(dst_ptr: u32, dst_len: u32) -> u32 {
    unsafe { copy_impl(dst_ptr as usize, dst_len as usize) as u32 }
}

/// Inner implementation using `usize` pointers — works on both wasm32 and
/// native 64-bit without truncation. Tests call this directly.
///
/// # Safety
/// `dst_ptr` must point to at least `dst_len` writable bytes.
pub(crate) unsafe fn copy_impl(dst_ptr: usize, dst_len: usize) -> usize {
    with(|s| {
        let n = std::cmp::min(s.len(), dst_len);
        if n > 0 {
            unsafe {
                std::ptr::copy_nonoverlapping(s.as_ptr(), dst_ptr as *mut u8, n);
            }
        }
        n
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn set_get_clear_roundtrips() {
        clear();
        assert_eq!(fulgur_last_error_len(), 0);
        set("boom");
        with(|s| assert_eq!(s, "boom"));
        assert_eq!(fulgur_last_error_len(), 4);
        clear();
        assert_eq!(fulgur_last_error_len(), 0);
    }

    #[test]
    fn copy_truncates_to_dst_len() {
        clear();
        set("0123456789");
        let mut buf = [0u8; 4];
        // Use copy_impl with usize ptr to avoid truncating a 64-bit pointer.
        let n = unsafe { copy_impl(buf.as_mut_ptr() as usize, buf.len()) };
        assert_eq!(n, 4);
        assert_eq!(&buf, b"0123");
    }
}
