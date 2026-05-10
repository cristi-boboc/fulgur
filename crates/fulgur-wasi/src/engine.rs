//! Engine state managed inside the WASM module.
//!
//! Engines live in a thread-local `Slab`, addressed by an opaque `u32`
//! handle returned to the host. Handle 0 is reserved as the sentinel for
//! "error / no handle".
//!
//! Each `EngineState` mirrors the deferred-config + assets pattern used by
//! `crates/fulgur-wasm::Engine`, so all options accumulate on the state
//! and the `fulgur::Engine` builder is constructed lazily at render time.

use crate::options::EngineOptions;
use fulgur::{AssetBundle, Margin, PageSize};
use slab::Slab;
use std::cell::RefCell;

#[derive(Default)]
pub(crate) struct EngineState {
    pub(crate) assets: AssetBundle,
    pub(crate) page_size: Option<PageSize>,
    pub(crate) margin: Option<Margin>,
    pub(crate) landscape: Option<bool>,
    pub(crate) title: Option<String>,
    pub(crate) authors: Vec<String>,
    pub(crate) description: Option<String>,
    pub(crate) keywords: Vec<String>,
    pub(crate) creator: Option<String>,
    pub(crate) producer: Option<String>,
    pub(crate) creation_date: Option<String>,
    pub(crate) lang: Option<String>,
    pub(crate) bookmarks: Option<bool>,
}

thread_local! {
    static ENGINES: RefCell<Slab<EngineState>> = RefCell::new(Slab::with_capacity(4));
}

/// Insert a new state and return its handle (slab index + 1; 0 is the
/// error sentinel).
pub(crate) fn insert(state: EngineState) -> u32 {
    ENGINES.with(|s| (s.borrow_mut().insert(state) as u32).wrapping_add(1))
}

/// Remove a handle from the slab.
pub(crate) fn remove(handle: u32) {
    if handle == 0 {
        return;
    }
    let key = (handle - 1) as usize;
    ENGINES.with(|s| {
        let mut slab = s.borrow_mut();
        if slab.contains(key) {
            slab.remove(key);
        }
    });
}

/// Run a closure with a mutable reference to the state for `handle`.
/// Returns `None` if the handle is unknown.
pub(crate) fn with_mut<R>(handle: u32, f: impl FnOnce(&mut EngineState) -> R) -> Option<R> {
    if handle == 0 {
        return None;
    }
    let key = (handle - 1) as usize;
    ENGINES.with(|s| {
        let mut slab = s.borrow_mut();
        slab.get_mut(key).map(f)
    })
}

/// Run a closure with an immutable reference to the state for `handle`.
pub(crate) fn with<R>(handle: u32, f: impl FnOnce(&EngineState) -> R) -> Option<R> {
    if handle == 0 {
        return None;
    }
    let key = (handle - 1) as usize;
    ENGINES.with(|s| {
        let slab = s.borrow();
        slab.get(key).map(f)
    })
}

pub(crate) fn apply_options(state: &mut EngineState, opts: EngineOptions) -> Result<(), String> {
    if let Some(ps) = opts.page_size {
        state.page_size = Some(ps.to_page_size()?);
    }
    if let Some(m) = opts.margin {
        state.margin = Some(m.to_margin());
    }
    if let Some(l) = opts.landscape {
        state.landscape = Some(l);
    }
    if let Some(t) = opts.title {
        state.title = Some(t);
    }
    if let Some(a) = opts.authors {
        state.authors = a;
    }
    if let Some(d) = opts.description {
        state.description = Some(d);
    }
    if let Some(k) = opts.keywords {
        state.keywords = k;
    }
    if let Some(c) = opts.creator {
        state.creator = Some(c);
    }
    if let Some(p) = opts.producer {
        state.producer = Some(p);
    }
    if let Some(cd) = opts.creation_date {
        state.creation_date = Some(cd);
    }
    if let Some(l) = opts.lang {
        state.lang = Some(l);
    }
    if let Some(b) = opts.bookmarks {
        state.bookmarks = Some(b);
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn insert_and_remove_handle() {
        let h = insert(EngineState::default());
        assert_ne!(h, 0);
        assert!(with(h, |_| ()).is_some());
        remove(h);
        assert!(with(h, |_| ()).is_none());
    }

    #[test]
    fn remove_zero_is_noop() {
        remove(0);
    }

    #[test]
    fn with_mut_can_mutate() {
        let h = insert(EngineState::default());
        with_mut(h, |s| s.title = Some("hi".into())).unwrap();
        let title = with(h, |s| s.title.clone()).unwrap();
        assert_eq!(title.as_deref(), Some("hi"));
        remove(h);
    }

    #[test]
    fn apply_options_partial_overrides() {
        let h = insert(EngineState::default());
        with_mut(h, |s| s.title = Some("kept".into())).unwrap();

        let opts: EngineOptions = serde_json::from_str(r#"{"landscape":true}"#).unwrap();
        with_mut(h, |s| super::apply_options(s, opts).unwrap()).unwrap();

        let (title, landscape) = with(h, |s| (s.title.clone(), s.landscape)).unwrap();
        assert_eq!(title.as_deref(), Some("kept"));
        assert_eq!(landscape, Some(true));
        remove(h);
    }
}
