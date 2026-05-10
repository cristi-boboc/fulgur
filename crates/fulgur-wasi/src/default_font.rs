//! Built-in default font registered automatically inside `engine_new`.
//!
//! Bundling Noto Sans Regular gives `Render` something to fall back to
//! when the host hasn't registered any custom fonts. Keep this file
//! tiny; license obligations are tracked in `examples/.fonts/OFL.txt`,
//! which the crate copies here for redistribution.

pub(crate) const NOTO_SANS_REGULAR: &[u8] = include_bytes!("../assets/NotoSans-Regular.ttf");
