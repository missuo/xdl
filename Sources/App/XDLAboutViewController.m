#import "XDLAboutViewController.h"
#import "XDLComponents.h"
#import "XDLTheme.h"

@implementation XDLAboutViewController

- (instancetype)init {
    return [super initWithTitle:@"About"];
}

- (void)viewDidLoad {
    [super viewDidLoad];

    NSStackView *stack = [self addCardWithCaption:nil
                                      belowAnchor:self.contentTopAnchor
                                              gap:6
                                          outCard:NULL];

    NSString *version = NSBundle.mainBundle.infoDictionary[@"CFBundleShortVersionString"] ?: @"dev";
    [stack addArrangedSubview:[XDLUI labelWithText:@"XDL"
                                              size:20 weight:NSFontWeightBold color:XDLTheme.primaryText]];
    [stack addArrangedSubview:[XDLUI labelWithText:[NSString stringWithFormat:@"Version %@", version]
                                              size:12 weight:NSFontWeightRegular color:XDLTheme.secondaryLabel]];
    [stack addArrangedSubview:[XDLUI labelWithText:@"Native AppKit rewrite of Twitter/X Media Batch Downloader.\nGo backend, Objective-C interface."
                                              size:12.5 weight:NSFontWeightRegular color:XDLTheme.label]];

    __weak typeof(self) weakSelf = self;
    XDLPillButton *original = [XDLPillButton pillWithTitle:@"Original Project (afkarxyz)" onClick:^{
        (void)weakSelf;
        [NSWorkspace.sharedWorkspace openURL:
            [NSURL URLWithString:@"https://github.com/afkarxyz/Twitter-X-Media-Batch-Downloader"]];
    }];
    [stack addArrangedSubview:original];
}

@end
