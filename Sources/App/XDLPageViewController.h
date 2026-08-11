#import <Cocoa/Cocoa.h>

NS_ASSUME_NONNULL_BEGIN

/// Base class for sidebar pages: fixed-position 28pt title over a transparent
/// scroll view with a flipped document view. Subclasses add content to
/// `contentView` and pin the first subview to `contentTopAnchor`.
@interface XDLPageViewController : NSViewController

@property (nonatomic, copy) NSString *pageTitle;

/// The flipped document view inside the scroll view.
@property (nonatomic, readonly) NSView *contentView;
/// Anchor just below the page title.
@property (nonatomic, readonly) NSLayoutYAxisAnchor *contentTopAnchor;
/// The scroll view's visible height — pin to this when a page must fill the
/// window rather than float at its minimum height.
@property (nonatomic, readonly) NSLayoutDimension *viewportHeightAnchor;
/// 28.
@property (nonatomic, readonly) CGFloat contentMargin;

- (instancetype)initWithTitle:(NSString *)title;

/// Convenience: adds a card containing a vertical stack with standard padding;
/// returns the stack for content.
- (NSStackView *)addCardWithCaption:(nullable NSString *)caption belowAnchor:(NSLayoutYAxisAnchor *)anchor gap:(CGFloat)gap outCard:(NSView *_Nullable *_Nullable)outCard;

@end

NS_ASSUME_NONNULL_END
