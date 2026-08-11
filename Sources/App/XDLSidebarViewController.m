#import "XDLSidebarViewController.h"
#import "XDLTheme.h"

@class XDLSidebarRow;

@interface XDLSidebarRow : NSView

@property (nonatomic, copy) NSString *itemID;
@property (nonatomic, copy) void (^onClick)(XDLSidebarRow *row);
@property (nonatomic) BOOL selected;
@property (nonatomic) BOOL hovered;
@property (nonatomic) NSTextField *label;

- (instancetype)initWithItemID:(NSString *)itemID title:(NSString *)title symbolName:(NSString *)symbolName;

@end

@implementation XDLSidebarRow

- (instancetype)initWithItemID:(NSString *)itemID title:(NSString *)title symbolName:(NSString *)symbolName {
    self = [super initWithFrame:NSZeroRect];
    if (self) {
        _itemID = [itemID copy];
        self.translatesAutoresizingMaskIntoConstraints = NO;
        self.wantsLayer = YES;
        self.layer.cornerRadius = 8.0;

        NSImageView *icon = [[NSImageView alloc] initWithFrame:NSZeroRect];
        NSImage *image = [NSImage imageWithSystemSymbolName:symbolName accessibilityDescription:title];
        NSImageSymbolConfiguration *config =
            [NSImageSymbolConfiguration configurationWithPointSize:14 weight:NSFontWeightMedium];
        icon.image = [image imageWithSymbolConfiguration:config];
        icon.contentTintColor = XDLTheme.label;
        icon.translatesAutoresizingMaskIntoConstraints = NO;
        [self addSubview:icon];

        _label = [NSTextField labelWithString:title];
        _label.font = [NSFont systemFontOfSize:14 weight:NSFontWeightMedium];
        _label.textColor = XDLTheme.sidebarTitle;
        _label.translatesAutoresizingMaskIntoConstraints = NO;
        [self addSubview:_label];

        [NSLayoutConstraint activateConstraints:@[
            [self.heightAnchor constraintEqualToConstant:34],
            [icon.leadingAnchor constraintEqualToAnchor:self.leadingAnchor constant:14],
            [icon.centerYAnchor constraintEqualToAnchor:self.centerYAnchor],
            [icon.widthAnchor constraintEqualToConstant:19],
            [icon.heightAnchor constraintEqualToConstant:19],
            [_label.leadingAnchor constraintEqualToAnchor:icon.trailingAnchor constant:10],
            [_label.centerYAnchor constraintEqualToAnchor:self.centerYAnchor],
            [_label.trailingAnchor constraintLessThanOrEqualToAnchor:self.trailingAnchor constant:-8],
        ]];

        [self addTrackingArea:[[NSTrackingArea alloc]
            initWithRect:NSZeroRect
                 options:NSTrackingMouseEnteredAndExited | NSTrackingActiveInKeyWindow | NSTrackingInVisibleRect
                   owner:self
                userInfo:nil]];
    }
    return self;
}

// Route child clicks to the row.
- (NSView *)hitTest:(NSPoint)point {
    NSView *view = [super hitTest:point];
    return view ? self : nil;
}

- (void)applyStyle {
    [self.effectiveAppearance performAsCurrentDrawingAppearance:^{
        if (self.selected) {
            self.layer.backgroundColor = XDLTheme.selectionBackground.CGColor;
        } else if (self.hovered) {
            self.layer.backgroundColor = XDLTheme.sidebarHover.CGColor;
        } else {
            self.layer.backgroundColor = NSColor.clearColor.CGColor;
        }
    }];
}

- (void)viewDidChangeEffectiveAppearance {
    [super viewDidChangeEffectiveAppearance];
    [self applyStyle];
}

- (void)setSelected:(BOOL)selected {
    _selected = selected;
    [self applyStyle];
}

- (void)mouseEntered:(NSEvent *)event {
    self.hovered = YES;
    [self applyStyle];
}

- (void)mouseExited:(NSEvent *)event {
    self.hovered = NO;
    [self applyStyle];
}

- (void)mouseDown:(NSEvent *)event {
    if (self.onClick) {
        self.onClick(self);
    }
}

- (BOOL)isAccessibilityElement { return YES; }
- (NSAccessibilityRole)accessibilityRole { return NSAccessibilityButtonRole; }
- (NSString *)accessibilityLabel { return self.label.stringValue; }
- (BOOL)accessibilityPerformPress {
    if (self.onClick) {
        self.onClick(self);
        return YES;
    }
    return NO;
}

@end

@interface XDLSidebarViewController ()
@property (nonatomic) NSStackView *stack;
@property (nonatomic) NSMutableArray<XDLSidebarRow *> *rows;
@end

@implementation XDLSidebarViewController

- (void)loadView {
    NSView *root = [[NSView alloc] initWithFrame:NSMakeRect(0, 0, XDLSidebarWidth, 640)];

    self.rows = [NSMutableArray array];
    self.stack = [[NSStackView alloc] initWithFrame:NSZeroRect];
    self.stack.orientation = NSUserInterfaceLayoutOrientationVertical;
    self.stack.alignment = NSLayoutAttributeLeading;
    self.stack.spacing = 2;
    self.stack.translatesAutoresizingMaskIntoConstraints = NO;
    [root addSubview:self.stack];

    [NSLayoutConstraint activateConstraints:@[
        [self.stack.topAnchor constraintEqualToAnchor:root.topAnchor constant:44],
        [self.stack.leadingAnchor constraintEqualToAnchor:root.leadingAnchor constant:8],
        [self.stack.trailingAnchor constraintEqualToAnchor:root.trailingAnchor constant:-8],
    ]];

    [self addItemWithID:@"fetch" title:@"Download" symbol:@"arrow.down.circle"];
    [self addItemWithID:@"library" title:@"Library" symbol:@"person.2"];

    [self addHeader:@"Tools"];
    [self addItemWithID:@"gifs" title:@"GIF Converter" symbol:@"film"];
    [self addItemWithID:@"dependencies" title:@"Dependencies" symbol:@"shippingbox"];

    [self addHeader:@"System"];
    [self addItemWithID:@"logs" title:@"Logs" symbol:@"text.alignleft"];
    [self addItemWithID:@"settings" title:@"Settings" symbol:@"gearshape"];
    [self addItemWithID:@"about" title:@"About" symbol:@"info.circle"];

    self.view = root;
}

- (void)addHeader:(NSString *)text {
    NSTextField *header = [NSTextField labelWithString:text.uppercaseString];
    header.font = [NSFont systemFontOfSize:11 weight:NSFontWeightSemibold];
    header.textColor = XDLTheme.sidebarHeader;
    header.translatesAutoresizingMaskIntoConstraints = NO;

    NSView *wrapper = [[NSView alloc] initWithFrame:NSZeroRect];
    wrapper.translatesAutoresizingMaskIntoConstraints = NO;
    [wrapper addSubview:header];
    [NSLayoutConstraint activateConstraints:@[
        [wrapper.heightAnchor constraintEqualToConstant:30],
        [header.leadingAnchor constraintEqualToAnchor:wrapper.leadingAnchor constant:14],
        [header.bottomAnchor constraintEqualToAnchor:wrapper.bottomAnchor constant:-4],
    ]];
    [self.stack addArrangedSubview:wrapper];
    [wrapper.widthAnchor constraintEqualToAnchor:self.stack.widthAnchor].active = YES;
}

- (void)addItemWithID:(NSString *)itemID title:(NSString *)title symbol:(NSString *)symbol {
    XDLSidebarRow *row = [[XDLSidebarRow alloc] initWithItemID:itemID title:title symbolName:symbol];
    __weak typeof(self) weakSelf = self;
    row.onClick = ^(XDLSidebarRow *clicked) {
        [weakSelf selectItemID:clicked.itemID];
    };
    [self.rows addObject:row];
    [self.stack addArrangedSubview:row];
    [row.widthAnchor constraintEqualToAnchor:self.stack.widthAnchor].active = YES;
}

- (void)selectItemID:(NSString *)itemID {
    for (XDLSidebarRow *row in self.rows) {
        row.selected = [row.itemID isEqualToString:itemID];
    }
    if (self.onSelect) {
        self.onSelect(itemID);
    }
}

@end
