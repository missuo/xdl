#import <Cocoa/Cocoa.h>

NS_ASSUME_NONNULL_BEGIN

/// 28pt rounded pill button with hover tracking and optional status dot.
@interface XDLPillButton : NSControl

@property (nonatomic, copy) NSString *title;
@property (nonatomic, copy, nullable) void (^onClick)(void);
@property (nonatomic, nullable) NSColor *dotColor; // nil hides the dot
/// Toggle-chip mode: selected shows the subtle selection fill.
@property (nonatomic) BOOL selected;

+ (instancetype)pillWithTitle:(NSString *)title onClick:(void (^_Nullable)(void))onClick;

@end

/// Small coloured status pill (text on tinted capsule) for table rows.
@interface XDLStatusPill : NSView

@property (nonatomic, copy) NSString *text;
@property (nonatomic) NSColor *color;

+ (instancetype)pill;

@end

/// Row view with a subtle rounded selection fill instead of the system blue
/// bar, so primary-colour text stays readable when selected.
@interface XDLTableRowView : NSTableRowView
@end

/// Table cell that vertically centres its single text label — a bare
/// NSTextField used as a cell view top-aligns its text within the row.
@interface XDLTableCell : NSView
@property (nonatomic, readonly) NSTextField *label;
@end

/// Factory helpers so pages stay terse.
@interface XDLUI : NSObject

+ (NSTextField *)labelWithText:(NSString *)text size:(CGFloat)size weight:(NSFontWeight)weight color:(NSColor *)color;
+ (NSTextField *)fieldWithPlaceholder:(NSString *)placeholder;
+ (NSSecureTextField *)secureFieldWithPlaceholder:(NSString *)placeholder;
+ (NSButton *)checkboxWithTitle:(NSString *)title target:(nullable id)target action:(nullable SEL)action;
+ (NSPopUpButton *)popupWithItems:(NSArray<NSString *> *)items;
+ (NSButton *)buttonWithTitle:(NSString *)title target:(nullable id)target action:(nullable SEL)action;
+ (NSView *)separator;

@end

NS_ASSUME_NONNULL_END
