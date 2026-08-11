#import "XDLLogsViewController.h"
#import "XDLCoreClient.h"
#import "XDLCardView.h"
#import "XDLTheme.h"

@interface XDLLogsViewController ()
@property (nonatomic) XDLCoreClient *client;
@property (nonatomic) NSTextView *textView;
@end

@implementation XDLLogsViewController

- (instancetype)initWithClient:(XDLCoreClient *)client {
    self = [super initWithTitle:@"Logs"];
    if (self) {
        _client = client;
    }
    return self;
}

- (void)viewDidLoad {
    [super viewDidLoad];

    XDLCardView *card = [[XDLCardView alloc] initWithFrame:NSZeroRect];
    card.cornerRadius = 12;
    [self.contentView addSubview:card];

    NSScrollView *scroll = [[NSScrollView alloc] initWithFrame:NSZeroRect];
    scroll.translatesAutoresizingMaskIntoConstraints = NO;
    scroll.drawsBackground = NO;
    scroll.hasVerticalScroller = YES;
    scroll.borderType = NSNoBorder;

    self.textView = [[NSTextView alloc] initWithFrame:NSZeroRect];
    self.textView.editable = NO;
    self.textView.drawsBackground = NO;
    self.textView.font = [NSFont monospacedSystemFontOfSize:11.5 weight:NSFontWeightRegular];
    self.textView.textColor = XDLTheme.label;
    self.textView.autoresizingMask = NSViewWidthSizable;
    self.textView.verticallyResizable = YES;
    self.textView.horizontallyResizable = NO;
    self.textView.textContainer.widthTracksTextView = YES;
    scroll.documentView = self.textView;
    [card addSubview:scroll];

    [NSLayoutConstraint activateConstraints:@[
        [card.topAnchor constraintEqualToAnchor:self.contentTopAnchor constant:6],
        [card.leadingAnchor constraintEqualToAnchor:self.contentView.leadingAnchor constant:self.contentMargin],
        [card.trailingAnchor constraintEqualToAnchor:self.contentView.trailingAnchor constant:-self.contentMargin],
        [card.heightAnchor constraintGreaterThanOrEqualToConstant:300],
        [self.contentView.heightAnchor constraintGreaterThanOrEqualToAnchor:self.viewportHeightAnchor],
        [card.bottomAnchor constraintEqualToAnchor:self.contentView.bottomAnchor constant:-self.contentMargin],

        [scroll.topAnchor constraintEqualToAnchor:card.topAnchor constant:10],
        [scroll.leadingAnchor constraintEqualToAnchor:card.leadingAnchor constant:12],
        [scroll.trailingAnchor constraintEqualToAnchor:card.trailingAnchor constant:-12],
        [scroll.bottomAnchor constraintEqualToAnchor:card.bottomAnchor constant:-10],
    ]];

    [self reloadAll];
    [NSNotificationCenter.defaultCenter addObserver:self
                                           selector:@selector(logLineArrived:)
                                               name:XDLCoreLogLineNotification
                                             object:nil];
}

- (void)dealloc {
    [NSNotificationCenter.defaultCenter removeObserver:self];
}

- (void)reloadAll {
    self.textView.string = [self.client.logLines componentsJoinedByString:@"\n"];
    [self scrollToEnd];
}

- (void)logLineArrived:(NSNotification *)notification {
    NSString *line = notification.userInfo[@"line"];
    if (![line isKindOfClass:NSString.class]) {
        return;
    }
    NSAttributedString *append = [[NSAttributedString alloc]
        initWithString:[(self.textView.string.length ? @"\n" : @"") stringByAppendingString:line]
            attributes:@{
                NSFontAttributeName: [NSFont monospacedSystemFontOfSize:11.5 weight:NSFontWeightRegular],
                NSForegroundColorAttributeName: XDLTheme.label,
            }];
    [self.textView.textStorage appendAttributedString:append];
    [self scrollToEnd];
}

- (void)scrollToEnd {
    [self.textView scrollRangeToVisible:NSMakeRange(self.textView.string.length, 0)];
}

@end
