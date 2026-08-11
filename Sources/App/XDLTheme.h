#import <Cocoa/Cocoa.h>

NS_ASSUME_NONNULL_BEGIN

/// Design tokens. Every colour is dynamic (light/dark in one code path).
/// Ported from the kasumi design system.
@interface XDLTheme : NSObject

+ (NSColor *)dynamicLight:(uint32_t)lightHex lightAlpha:(CGFloat)lightAlpha
                     dark:(uint32_t)darkHex darkAlpha:(CGFloat)darkAlpha;

// Backdrop gradient (315°, stops 0.0 / 0.5 / 1.0)
+ (NSArray<NSColor *> *)backdropColors;

// Surfaces
+ (NSColor *)cardBackground;
+ (NSColor *)controlBackground;

// Text
+ (NSColor *)primaryText;
+ (NSColor *)label;
+ (NSColor *)secondaryLabel;
+ (NSColor *)sidebarTitle;
+ (NSColor *)sidebarHeader;

// Lines
+ (NSColor *)border;
+ (NSColor *)separator;

// Accents (headings and data only — never control fills)
+ (NSColor *)accentBlue;
+ (NSColor *)accentCyan;
+ (NSColor *)accentPurple;
+ (NSColor *)accentMagenta;
+ (NSColor *)accentIndigo;

// Interaction
+ (NSColor *)buttonHovered;
+ (NSColor *)buttonHighlighted;
+ (NSColor *)selectionBackground;
+ (NSColor *)sidebarHover;

// Status
+ (NSColor *)statusGreen;
+ (NSColor *)statusOrange;
+ (NSColor *)statusRed;

@end

// Shared metrics
extern const CGFloat XDLPageTitleFontSize;   // 28
extern const CGFloat XDLPageTitleTopInset;   // 52
extern const CGFloat XDLPageContentGap;      // 14
extern const CGFloat XDLPageContentMargin;   // 28
extern const CGFloat XDLCardCornerRadius;    // 18
extern const CGFloat XDLCardPadding;         // 18
extern const CGFloat XDLSidebarWidth;        // 208

NS_ASSUME_NONNULL_END
