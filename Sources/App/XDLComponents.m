#import "XDLComponents.h"
#import "XDLTheme.h"

@interface XDLPillButton ()
@property (nonatomic) NSTextField *labelView;
@property (nonatomic) NSView *dotView;
@property (nonatomic) BOOL hovered;
@property (nonatomic) NSStackView *stack;
@end

@implementation XDLPillButton

+ (instancetype)pillWithTitle:(NSString *)title onClick:(void (^)(void))onClick {
    XDLPillButton *button = [[self alloc] initWithFrame:NSZeroRect];
    button.title = title;
    button.onClick = onClick;
    return button;
}

- (instancetype)initWithFrame:(NSRect)frameRect {
    self = [super initWithFrame:frameRect];
    if (self) {
        self.translatesAutoresizingMaskIntoConstraints = NO;
        self.wantsLayer = YES;
        self.layer.cornerRadius = 14.0;

        _dotView = [[NSView alloc] initWithFrame:NSZeroRect];
        _dotView.translatesAutoresizingMaskIntoConstraints = NO;
        _dotView.wantsLayer = YES;
        _dotView.layer.cornerRadius = 3.5;
        _dotView.hidden = YES;
        [NSLayoutConstraint activateConstraints:@[
            [_dotView.widthAnchor constraintEqualToConstant:7],
            [_dotView.heightAnchor constraintEqualToConstant:7],
        ]];

        _labelView = [NSTextField labelWithString:@""];
        _labelView.font = [NSFont systemFontOfSize:12 weight:NSFontWeightMedium];
        _labelView.textColor = XDLTheme.label;

        _stack = [NSStackView stackViewWithViews:@[ _dotView, _labelView ]];
        _stack.orientation = NSUserInterfaceLayoutOrientationHorizontal;
        _stack.spacing = 6;
        _stack.detachesHiddenViews = YES;
        _stack.translatesAutoresizingMaskIntoConstraints = NO;
        [self addSubview:_stack];
        // Pin both sides so the pill hugs its content symmetrically — a
        // centerX + derived-width setup let stray constraints widen the pill
        // with the text stuck to one side.
        [NSLayoutConstraint activateConstraints:@[
            [self.heightAnchor constraintEqualToConstant:28],
            [_stack.leadingAnchor constraintEqualToAnchor:self.leadingAnchor constant:12],
            [_stack.trailingAnchor constraintEqualToAnchor:self.trailingAnchor constant:-12],
            [_stack.centerYAnchor constraintEqualToAnchor:self.centerYAnchor],
        ]];
        [self setContentHuggingPriority:NSLayoutPriorityDefaultHigh
                         forOrientation:NSLayoutConstraintOrientationHorizontal];

        [self addTrackingArea:[[NSTrackingArea alloc]
            initWithRect:NSZeroRect
                 options:NSTrackingMouseEnteredAndExited | NSTrackingActiveInKeyWindow | NSTrackingInVisibleRect
                   owner:self
                userInfo:nil]];
        [self applyStyle];
    }
    return self;
}

- (void)setTitle:(NSString *)title {
    _title = [title copy];
    self.labelView.stringValue = title;
}

- (void)setDotColor:(NSColor *)dotColor {
    _dotColor = dotColor;
    self.dotView.hidden = (dotColor == nil);
    [self applyStyle];
}

- (void)setSelected:(BOOL)selected {
    _selected = selected;
    [self applyStyle];
}

- (void)setEnabled:(BOOL)enabled {
    [super setEnabled:enabled];
    self.alphaValue = enabled ? 1.0 : 0.45;
}

- (void)applyStyle {
    [self.effectiveAppearance performAsCurrentDrawingAppearance:^{
        NSColor *fill = XDLTheme.controlBackground;
        if (self.hovered) {
            fill = XDLTheme.buttonHovered;
        } else if (self.selected) {
            fill = XDLTheme.selectionBackground;
        }
        self.layer.backgroundColor = fill.CGColor;
        self.layer.borderWidth = 0.5;
        self.layer.borderColor = XDLTheme.separator.CGColor;
        if (self.dotColor) {
            self.dotView.layer.backgroundColor = self.dotColor.CGColor;
        }
    }];
}

- (void)viewDidChangeEffectiveAppearance {
    [super viewDidChangeEffectiveAppearance];
    [self applyStyle];
}

- (void)mouseEntered:(NSEvent *)event {
    if (!self.enabled) return;
    self.hovered = YES;
    [self applyStyle];
}

- (void)mouseExited:(NSEvent *)event {
    self.hovered = NO;
    [self applyStyle];
}

- (void)mouseDown:(NSEvent *)event {
    if (!self.enabled) return;
    [self.effectiveAppearance performAsCurrentDrawingAppearance:^{
        self.layer.backgroundColor = XDLTheme.buttonHighlighted.CGColor;
    }];
}

- (void)mouseUp:(NSEvent *)event {
    if (!self.enabled) return;
    [self applyStyle];
    NSPoint point = [self convertPoint:event.locationInWindow fromView:nil];
    if (NSPointInRect(point, self.bounds) && self.onClick) {
        self.onClick();
    }
}

- (BOOL)isAccessibilityElement { return YES; }
- (NSAccessibilityRole)accessibilityRole { return NSAccessibilityButtonRole; }
- (NSString *)accessibilityLabel { return self.title; }
- (BOOL)accessibilityPerformPress {
    if (self.enabled && self.onClick) {
        self.onClick();
        return YES;
    }
    return NO;
}

@end

@interface XDLStatusPill ()
@property (nonatomic) NSTextField *labelView;
@end

@implementation XDLStatusPill

+ (instancetype)pill {
    return [[self alloc] initWithFrame:NSZeroRect];
}

- (instancetype)initWithFrame:(NSRect)frameRect {
    self = [super initWithFrame:frameRect];
    if (self) {
        self.translatesAutoresizingMaskIntoConstraints = NO;
        self.wantsLayer = YES;
        self.layer.cornerRadius = 8.0;
        _color = XDLTheme.secondaryLabel;

        _labelView = [NSTextField labelWithString:@""];
        _labelView.font = [NSFont systemFontOfSize:10.5 weight:NSFontWeightSemibold];
        _labelView.translatesAutoresizingMaskIntoConstraints = NO;
        [self addSubview:_labelView];
        [NSLayoutConstraint activateConstraints:@[
            [self.heightAnchor constraintEqualToConstant:16],
            [_labelView.centerXAnchor constraintEqualToAnchor:self.centerXAnchor],
            [_labelView.centerYAnchor constraintEqualToAnchor:self.centerYAnchor],
            [self.widthAnchor constraintEqualToAnchor:_labelView.widthAnchor constant:14],
        ]];
    }
    return self;
}

- (void)setText:(NSString *)text {
    _text = [text copy];
    self.labelView.stringValue = text;
    [self applyStyle];
}

- (void)setColor:(NSColor *)color {
    _color = color;
    [self applyStyle];
}

- (void)applyStyle {
    self.labelView.textColor = self.color;
    [self.effectiveAppearance performAsCurrentDrawingAppearance:^{
        self.layer.backgroundColor = [self.color colorWithAlphaComponent:0.14].CGColor;
    }];
}

- (void)viewDidChangeEffectiveAppearance {
    [super viewDidChangeEffectiveAppearance];
    [self applyStyle];
}

@end

@implementation XDLTableRowView

- (void)drawSelectionInRect:(NSRect)dirtyRect {
    if (self.selectionHighlightStyle == NSTableViewSelectionHighlightStyleNone) {
        return;
    }
    NSRect rect = NSInsetRect(self.bounds, 4, 1);
    NSBezierPath *path = [NSBezierPath bezierPathWithRoundedRect:rect xRadius:6 yRadius:6];
    [XDLTheme.selectionBackground setFill];
    [path fill];
}

// Keep cell text in its normal colours — the subtle fill needs no white text.
- (NSBackgroundStyle)interiorBackgroundStyle {
    return NSBackgroundStyleNormal;
}

@end

@implementation XDLTableCell

- (instancetype)initWithFrame:(NSRect)frameRect {
    self = [super initWithFrame:frameRect];
    if (self) {
        _label = [XDLUI labelWithText:@"" size:12 weight:NSFontWeightRegular color:XDLTheme.label];
        [self addSubview:_label];
        [NSLayoutConstraint activateConstraints:@[
            [_label.leadingAnchor constraintEqualToAnchor:self.leadingAnchor],
            [_label.trailingAnchor constraintLessThanOrEqualToAnchor:self.trailingAnchor],
            [_label.centerYAnchor constraintEqualToAnchor:self.centerYAnchor],
        ]];
    }
    return self;
}

@end

@implementation XDLUI

+ (NSTextField *)labelWithText:(NSString *)text size:(CGFloat)size weight:(NSFontWeight)weight color:(NSColor *)color {
    NSTextField *label = [NSTextField labelWithString:text];
    label.font = [NSFont systemFontOfSize:size weight:weight];
    label.textColor = color;
    label.translatesAutoresizingMaskIntoConstraints = NO;
    // A label's intrinsic width is its whole string — allow compression or one
    // long value widens the window.
    [label setContentCompressionResistancePriority:NSLayoutPriorityDefaultLow
                                    forOrientation:NSLayoutConstraintOrientationHorizontal];
    label.lineBreakMode = NSLineBreakByTruncatingTail;
    return label;
}

+ (NSTextField *)fieldWithPlaceholder:(NSString *)placeholder {
    NSTextField *field = [[NSTextField alloc] initWithFrame:NSZeroRect];
    field.placeholderString = placeholder;
    field.font = [NSFont systemFontOfSize:12.5];
    field.bezelStyle = NSTextFieldRoundedBezel;
    field.translatesAutoresizingMaskIntoConstraints = NO;
    return field;
}

+ (NSSecureTextField *)secureFieldWithPlaceholder:(NSString *)placeholder {
    NSSecureTextField *field = [[NSSecureTextField alloc] initWithFrame:NSZeroRect];
    field.placeholderString = placeholder;
    field.font = [NSFont systemFontOfSize:12.5];
    field.bezelStyle = NSTextFieldRoundedBezel;
    field.translatesAutoresizingMaskIntoConstraints = NO;
    return field;
}

+ (NSButton *)checkboxWithTitle:(NSString *)title target:(id)target action:(SEL)action {
    NSButton *box = [NSButton checkboxWithTitle:title target:target action:action];
    box.font = [NSFont systemFontOfSize:12];
    box.translatesAutoresizingMaskIntoConstraints = NO;
    return box;
}

+ (NSPopUpButton *)popupWithItems:(NSArray<NSString *> *)items {
    NSPopUpButton *popup = [[NSPopUpButton alloc] initWithFrame:NSZeroRect pullsDown:NO];
    [popup addItemsWithTitles:items];
    popup.font = [NSFont systemFontOfSize:12];
    popup.translatesAutoresizingMaskIntoConstraints = NO;
    return popup;
}

+ (NSButton *)buttonWithTitle:(NSString *)title target:(id)target action:(SEL)action {
    NSButton *button = [NSButton buttonWithTitle:title target:target action:action];
    button.bezelStyle = NSBezelStyleRounded;
    button.translatesAutoresizingMaskIntoConstraints = NO;
    return button;
}

+ (NSView *)separator {
    NSBox *box = [[NSBox alloc] initWithFrame:NSZeroRect];
    box.boxType = NSBoxSeparator;
    box.translatesAutoresizingMaskIntoConstraints = NO;
    return box;
}

@end
