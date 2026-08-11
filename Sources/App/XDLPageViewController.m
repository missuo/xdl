#import "XDLPageViewController.h"
#import "XDLCardView.h"
#import "XDLTheme.h"

@interface XDLFlippedDocView : NSView
@end

@implementation XDLFlippedDocView
- (BOOL)isFlipped { return YES; }
@end

@interface XDLPageViewController ()
@property (nonatomic) NSTextField *titleLabel;
@property (nonatomic) NSScrollView *scrollView;
@property (nonatomic) NSView *docView;
@end

@implementation XDLPageViewController

- (instancetype)initWithTitle:(NSString *)title {
    self = [super initWithNibName:nil bundle:nil];
    if (self) {
        _pageTitle = [title copy];
    }
    return self;
}

- (void)loadView {
    NSView *root = [[NSView alloc] initWithFrame:NSMakeRect(0, 0, 700, 640)];

    _titleLabel = [NSTextField labelWithString:self.pageTitle ?: @""];
    _titleLabel.font = [NSFont systemFontOfSize:XDLPageTitleFontSize weight:NSFontWeightBold];
    _titleLabel.textColor = XDLTheme.primaryText;
    _titleLabel.translatesAutoresizingMaskIntoConstraints = NO;
    [root addSubview:_titleLabel];

    _docView = [[XDLFlippedDocView alloc] initWithFrame:NSZeroRect];
    _docView.translatesAutoresizingMaskIntoConstraints = NO;

    _scrollView = [[NSScrollView alloc] initWithFrame:NSZeroRect];
    _scrollView.translatesAutoresizingMaskIntoConstraints = NO;
    _scrollView.drawsBackground = NO;
    _scrollView.hasVerticalScroller = YES;
    _scrollView.verticalScrollElasticity = NSScrollElasticityAllowed;
    // Mandatory under FullSizeContentView, or AppKit insets the document by
    // the titlebar height.
    _scrollView.automaticallyAdjustsContentInsets = NO;
    _scrollView.contentInsets = NSEdgeInsetsZero;
    _scrollView.documentView = _docView;
    [root addSubview:_scrollView];

    [NSLayoutConstraint activateConstraints:@[
        [_titleLabel.topAnchor constraintEqualToAnchor:root.topAnchor constant:XDLPageTitleTopInset],
        [_titleLabel.leadingAnchor constraintEqualToAnchor:root.leadingAnchor constant:XDLPageContentMargin],

        [_scrollView.topAnchor constraintEqualToAnchor:_titleLabel.bottomAnchor constant:XDLPageContentGap],
        [_scrollView.leadingAnchor constraintEqualToAnchor:root.leadingAnchor],
        [_scrollView.trailingAnchor constraintEqualToAnchor:root.trailingAnchor],
        [_scrollView.bottomAnchor constraintEqualToAnchor:root.bottomAnchor],

        [_docView.widthAnchor constraintEqualToAnchor:_scrollView.widthAnchor],
    ]];

    self.view = root;
}

- (NSView *)contentView { return self.docView; }
- (NSLayoutYAxisAnchor *)contentTopAnchor { return self.docView.topAnchor; }
- (NSLayoutDimension *)viewportHeightAnchor { return self.scrollView.heightAnchor; }
- (CGFloat)contentMargin { return XDLPageContentMargin; }

- (NSStackView *)addCardWithCaption:(NSString *)caption belowAnchor:(NSLayoutYAxisAnchor *)anchor gap:(CGFloat)gap outCard:(NSView **)outCard {
    NSLayoutYAxisAnchor *topAnchor = anchor;

    if (caption.length > 0) {
        XDLCaptionLabel *cap = [XDLCaptionLabel captionWithText:caption];
        [self.contentView addSubview:cap];
        [NSLayoutConstraint activateConstraints:@[
            [cap.topAnchor constraintEqualToAnchor:anchor constant:gap],
            [cap.leadingAnchor constraintEqualToAnchor:self.contentView.leadingAnchor constant:XDLPageContentMargin + 4],
        ]];
        topAnchor = cap.bottomAnchor;
        gap = 8;
    }

    XDLCardView *card = [[XDLCardView alloc] initWithFrame:NSZeroRect];
    [self.contentView addSubview:card];

    NSStackView *stack = [[NSStackView alloc] initWithFrame:NSZeroRect];
    stack.orientation = NSUserInterfaceLayoutOrientationVertical;
    stack.alignment = NSLayoutAttributeLeading;
    stack.spacing = 12;
    stack.translatesAutoresizingMaskIntoConstraints = NO;
    [card addSubview:stack];

    [NSLayoutConstraint activateConstraints:@[
        [card.topAnchor constraintEqualToAnchor:topAnchor constant:gap],
        [card.leadingAnchor constraintEqualToAnchor:self.contentView.leadingAnchor constant:XDLPageContentMargin],
        [card.trailingAnchor constraintEqualToAnchor:self.contentView.trailingAnchor constant:-XDLPageContentMargin],

        [stack.topAnchor constraintEqualToAnchor:card.topAnchor constant:XDLCardPadding],
        [stack.leadingAnchor constraintEqualToAnchor:card.leadingAnchor constant:XDLCardPadding],
        [stack.trailingAnchor constraintEqualToAnchor:card.trailingAnchor constant:-XDLCardPadding],
        [stack.bottomAnchor constraintEqualToAnchor:card.bottomAnchor constant:-XDLCardPadding],
    ]];

    if (outCard) {
        *outCard = card;
    }
    return stack;
}

@end
