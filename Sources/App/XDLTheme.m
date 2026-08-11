#import "XDLTheme.h"

const CGFloat XDLPageTitleFontSize = 28.0;
const CGFloat XDLPageTitleTopInset = 52.0;
const CGFloat XDLPageContentGap = 14.0;
const CGFloat XDLPageContentMargin = 28.0;
const CGFloat XDLCardCornerRadius = 18.0;
const CGFloat XDLCardPadding = 18.0;
const CGFloat XDLSidebarWidth = 208.0;

static NSColor *ColorFromHex(uint32_t hex, CGFloat alpha) {
    return [NSColor colorWithSRGBRed:((hex >> 16) & 0xFF) / 255.0
                               green:((hex >> 8) & 0xFF) / 255.0
                                blue:(hex & 0xFF) / 255.0
                               alpha:alpha];
}

@implementation XDLTheme

+ (NSColor *)dynamicLight:(uint32_t)lightHex lightAlpha:(CGFloat)lightAlpha
                     dark:(uint32_t)darkHex darkAlpha:(CGFloat)darkAlpha {
    return [NSColor colorWithName:nil dynamicProvider:^NSColor *(NSAppearance *appearance) {
        NSAppearanceName name = [appearance bestMatchFromAppearancesWithNames:@[
            NSAppearanceNameAqua, NSAppearanceNameDarkAqua
        ]];
        if ([name isEqualToString:NSAppearanceNameDarkAqua]) {
            return ColorFromHex(darkHex, darkAlpha);
        }
        return ColorFromHex(lightHex, lightAlpha);
    }];
}

+ (NSArray<NSColor *> *)backdropColors {
    return @[
        [self dynamicLight:0xEAF0FA lightAlpha:1 dark:0x232936 darkAlpha:1],
        [self dynamicLight:0xEFF0F6 lightAlpha:1 dark:0x24242C darkAlpha:1],
        [self dynamicLight:0xF6F4F7 lightAlpha:1 dark:0x201F26 darkAlpha:1],
    ];
}

+ (NSColor *)cardBackground {
    return [self dynamicLight:0xFFFFFF lightAlpha:0.80 dark:0x000000 darkAlpha:0.20];
}

+ (NSColor *)controlBackground {
    return [self dynamicLight:0xFFFFFF lightAlpha:0.80 dark:0x000000 darkAlpha:0.20];
}

+ (NSColor *)primaryText {
    return [self dynamicLight:0x131417 lightAlpha:1 dark:0xEAEAEA darkAlpha:1];
}

+ (NSColor *)label {
    return [self dynamicLight:0x202327 lightAlpha:1 dark:0xDDDDDD darkAlpha:1];
}

+ (NSColor *)secondaryLabel {
    return [self dynamicLight:0xB3B3B3 lightAlpha:1 dark:0x7F838F darkAlpha:1];
}

+ (NSColor *)sidebarTitle {
    return [self primaryText];
}

+ (NSColor *)sidebarHeader {
    return [self dynamicLight:0x131417 lightAlpha:0.5 dark:0xEAEAEA darkAlpha:0.5];
}

+ (NSColor *)border {
    return [self dynamicLight:0xE8E2E0 lightAlpha:1 dark:0x534B44 darkAlpha:1];
}

+ (NSColor *)separator {
    return [self dynamicLight:0x000000 lightAlpha:0.12 dark:0xFFFFFF darkAlpha:0.12];
}

+ (NSColor *)accentBlue { return ColorFromHex(0x0CBEFF, 1); }
+ (NSColor *)accentCyan { return ColorFromHex(0x20CDCD, 1); }
+ (NSColor *)accentPurple { return ColorFromHex(0x6C5CE7, 1); }
+ (NSColor *)accentMagenta { return ColorFromHex(0xCC47CC, 1); }
+ (NSColor *)accentIndigo { return ColorFromHex(0x7B61FF, 1); }

+ (NSColor *)buttonHovered {
    return [self dynamicLight:0x000000 lightAlpha:0.10 dark:0xFFFFFF darkAlpha:0.10];
}

+ (NSColor *)buttonHighlighted {
    return [self dynamicLight:0x000000 lightAlpha:0.15 dark:0xFFFFFF darkAlpha:0.15];
}

+ (NSColor *)selectionBackground {
    return [self dynamicLight:0x000000 lightAlpha:0.085 dark:0xFFFFFF darkAlpha:0.11];
}

+ (NSColor *)sidebarHover {
    return [self dynamicLight:0x000000 lightAlpha:0.045 dark:0xFFFFFF darkAlpha:0.06];
}

+ (NSColor *)statusGreen { return ColorFromHex(0x34C759, 1); }
+ (NSColor *)statusOrange { return ColorFromHex(0xFF9500, 1); }
+ (NSColor *)statusRed { return ColorFromHex(0xFF3B30, 1); }

@end
