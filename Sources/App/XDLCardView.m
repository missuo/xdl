#import "XDLCardView.h"
#import "XDLTheme.h"

@implementation XDLCardView

- (instancetype)initWithFrame:(NSRect)frameRect {
    self = [super initWithFrame:frameRect];
    if (self) {
        _cornerRadius = XDLCardCornerRadius;
        self.wantsLayer = YES;
        self.translatesAutoresizingMaskIntoConstraints = NO;
        [self applyLayerStyle];
    }
    return self;
}

- (void)applyLayerStyle {
    // CGColor does not follow the appearance automatically — resolve against
    // this view's effective appearance and re-apply on change.
    [self.effectiveAppearance performAsCurrentDrawingAppearance:^{
        self.layer.cornerRadius = self.cornerRadius;
        self.layer.backgroundColor = XDLTheme.cardBackground.CGColor;
        self.layer.borderWidth = 0.5;
        self.layer.borderColor = [NSColor colorWithWhite:1.0 alpha:0.5].CGColor;
        self.layer.masksToBounds = NO;
        self.layer.shadowColor = NSColor.blackColor.CGColor;
        self.layer.shadowOpacity = 0.10;
        self.layer.shadowRadius = 14.0;
        self.layer.shadowOffset = CGSizeMake(0, -3);
    }];
}

- (void)viewDidChangeEffectiveAppearance {
    [super viewDidChangeEffectiveAppearance];
    [self applyLayerStyle];
}

@end

@implementation XDLCaptionLabel

+ (instancetype)captionWithText:(NSString *)text {
    XDLCaptionLabel *label = [self labelWithString:text.uppercaseString];
    label.font = [NSFont systemFontOfSize:11 weight:NSFontWeightSemibold];
    label.textColor = XDLTheme.secondaryLabel;
    label.translatesAutoresizingMaskIntoConstraints = NO;
    return label;
}

@end
