#import <Cocoa/Cocoa.h>

NS_ASSUME_NONNULL_BEGIN

/// Translucent rounded card with hairline border and soft shadow.
@interface XDLCardView : NSView

/// Corner radius, default 18. Set before display.
@property (nonatomic) CGFloat cornerRadius;

@end

/// 11pt semibold uppercase caption used above sections and form fields.
@interface XDLCaptionLabel : NSTextField

+ (instancetype)captionWithText:(NSString *)text;

@end

NS_ASSUME_NONNULL_END
