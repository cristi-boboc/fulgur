//! Engine configuration options accepted via the JSON `configure` call.
//!
//! The shape is identical to the `EngineOptions` struct in
//! `crates/fulgur-wasm/src/lib.rs` — kept in sync by hand. Both crates
//! talk to `fulgur::Engine::builder()` through the same field set, so
//! drift here is a binding bug.

use fulgur::{Margin, PageSize};
use serde::Deserialize;

#[derive(Deserialize, Default, Debug)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub(crate) struct EngineOptions {
    #[serde(default)]
    pub page_size: Option<PageSizeOption>,
    #[serde(default)]
    pub margin: Option<MarginOption>,
    #[serde(default)]
    pub landscape: Option<bool>,
    #[serde(default)]
    pub title: Option<String>,
    #[serde(default)]
    pub authors: Option<Vec<String>>,
    #[serde(default)]
    pub description: Option<String>,
    #[serde(default)]
    pub keywords: Option<Vec<String>>,
    #[serde(default)]
    pub creator: Option<String>,
    #[serde(default)]
    pub producer: Option<String>,
    #[serde(default)]
    pub creation_date: Option<String>,
    #[serde(default)]
    pub lang: Option<String>,
    #[serde(default)]
    pub bookmarks: Option<bool>,
}

#[derive(Deserialize, Debug)]
#[serde(untagged)]
pub(crate) enum PageSizeOption {
    Named(String),
    #[serde(rename_all = "camelCase")]
    Custom { width_mm: f32, height_mm: f32 },
}

#[derive(Deserialize, Debug)]
#[serde(untagged)]
pub(crate) enum MarginOption {
    Mm { mm: f32 },
    Pt { pt: f32 },
    #[serde(rename_all = "camelCase")]
    Full {
        top_mm: f32,
        right_mm: f32,
        bottom_mm: f32,
        left_mm: f32,
    },
}

impl PageSizeOption {
    pub(crate) fn to_page_size(&self) -> Result<PageSize, String> {
        match self {
            Self::Named(name) => match name.to_ascii_lowercase().as_str() {
                "a4" => Ok(PageSize::A4),
                "a3" => Ok(PageSize::A3),
                "letter" => Ok(PageSize::LETTER),
                other => Err(format!("unknown page size: {other}")),
            },
            Self::Custom { width_mm, height_mm } => Ok(PageSize::custom(*width_mm, *height_mm)),
        }
    }
}

impl MarginOption {
    pub(crate) fn to_margin(&self) -> Margin {
        match self {
            Self::Mm { mm } => Margin::uniform_mm(*mm),
            Self::Pt { pt } => Margin::uniform(*pt),
            Self::Full {
                top_mm,
                right_mm,
                bottom_mm,
                left_mm,
            } => {
                let to_pt = |mm: f32| mm * 72.0 / 25.4;
                Margin {
                    top: to_pt(*top_mm),
                    right: to_pt(*right_mm),
                    bottom: to_pt(*bottom_mm),
                    left: to_pt(*left_mm),
                }
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parses_named_page_size_and_landscape() {
        let opts: EngineOptions =
            serde_json::from_str(r#"{"pageSize":"A4","landscape":true}"#).unwrap();
        assert!(matches!(opts.page_size, Some(PageSizeOption::Named(ref s)) if s == "A4"));
        assert_eq!(opts.landscape, Some(true));
    }

    #[test]
    fn parses_custom_page_size() {
        let opts: EngineOptions =
            serde_json::from_str(r#"{"pageSize":{"widthMm":100.0,"heightMm":150.0}}"#).unwrap();
        match opts.page_size.unwrap() {
            PageSizeOption::Custom { width_mm, height_mm } => {
                assert_eq!(width_mm, 100.0);
                assert_eq!(height_mm, 150.0);
            }
            _ => panic!("expected Custom variant"),
        }
    }

    #[test]
    fn parses_margin_variants() {
        let mm: EngineOptions = serde_json::from_str(r#"{"margin":{"mm":10.0}}"#).unwrap();
        assert!(matches!(mm.margin, Some(MarginOption::Mm { mm }) if mm == 10.0));

        let pt: EngineOptions = serde_json::from_str(r#"{"margin":{"pt":36.0}}"#).unwrap();
        assert!(matches!(pt.margin, Some(MarginOption::Pt { pt }) if pt == 36.0));

        let full: EngineOptions = serde_json::from_str(
            r#"{"margin":{"topMm":1.0,"rightMm":2.0,"bottomMm":3.0,"leftMm":4.0}}"#,
        )
        .unwrap();
        assert!(matches!(full.margin, Some(MarginOption::Full { .. })));
    }

    #[test]
    fn rejects_unknown_top_level_field() {
        let err = serde_json::from_str::<EngineOptions>(r#"{"unknownField":1}"#).unwrap_err();
        assert!(err.to_string().contains("unknown field"), "got: {err}");
    }

    #[test]
    fn rejects_unknown_page_size_name() {
        let opts: EngineOptions = serde_json::from_str(r#"{"pageSize":"foo"}"#).unwrap();
        let err = opts.page_size.unwrap().to_page_size().unwrap_err();
        assert!(err.contains("unknown page size"));
    }
}
