#import "XDLGifsViewController.h"
#import "XDLCoreClient.h"
#import "XDLComponents.h"
#import "XDLSettings.h"
#import "XDLTheme.h"

@interface XDLGifsViewController ()
@property (nonatomic) XDLCoreClient *client;
@property (nonatomic) NSTextField *folderField;
@property (nonatomic) NSPopUpButton *qualityPopup;
@property (nonatomic) NSPopUpButton *resolutionPopup;
@property (nonatomic) NSButton *deleteOriginalBox;
@property (nonatomic) XDLPillButton *convertButton;
@property (nonatomic) NSProgressIndicator *spinner;
@property (nonatomic) NSTextField *statusLabel;
@property (nonatomic) BOOL converting;
@end

@implementation XDLGifsViewController

- (instancetype)initWithClient:(XDLCoreClient *)client {
    self = [super initWithTitle:@"GIF Converter"];
    if (self) {
        _client = client;
    }
    return self;
}

- (void)viewDidLoad {
    [super viewDidLoad];

    NSStackView *stack = [self addCardWithCaption:@"Convert MP4 → GIF"
                                      belowAnchor:self.contentTopAnchor
                                              gap:6
                                          outCard:NULL];

    NSTextField *blurb = [XDLUI labelWithText:
        @"X serves animated GIFs as .mp4 files. Pick a folder (e.g. a downloaded account's gifs folder) to convert them into real .gif files with FFmpeg."
                                         size:11.5
                                       weight:NSFontWeightRegular
                                        color:XDLTheme.secondaryLabel];
    blurb.lineBreakMode = NSLineBreakByWordWrapping;
    blurb.maximumNumberOfLines = 0;
    [stack addArrangedSubview:blurb];
    [blurb.widthAnchor constraintEqualToAnchor:stack.widthAnchor].active = YES;

    self.folderField = [XDLUI fieldWithPlaceholder:@"Folder containing .mp4 files"];
    self.folderField.editable = NO;
    __weak typeof(self) weakSelf = self;
    XDLPillButton *browse = [XDLPillButton pillWithTitle:@"Choose…" onClick:^{
        [weakSelf chooseFolder];
    }];
    [self.folderField setContentHuggingPriority:100
                                 forOrientation:NSLayoutConstraintOrientationHorizontal];
    NSStackView *folderRow = [NSStackView stackViewWithViews:@[ self.folderField, browse ]];
    folderRow.spacing = 10;
    folderRow.distribution = NSStackViewDistributionFill;
    [stack addArrangedSubview:folderRow];
    [folderRow.widthAnchor constraintEqualToAnchor:stack.widthAnchor].active = YES;

    XDLSettings *settings = XDLSettings.shared;
    self.qualityPopup = [XDLUI popupWithItems:@[ @"Fast", @"Better" ]];
    [self.qualityPopup selectItemAtIndex:[settings.gifQuality isEqualToString:@"better"] ? 1 : 0];
    self.resolutionPopup = [XDLUI popupWithItems:@[ @"Original", @"High (800px)", @"Medium (600px)", @"Low (400px)" ]];
    NSArray *resolutions = @[ @"original", @"high", @"medium", @"low" ];
    NSUInteger resolutionIndex = [resolutions indexOfObject:settings.gifResolution];
    [self.resolutionPopup selectItemAtIndex:resolutionIndex == NSNotFound ? 0 : resolutionIndex];
    self.deleteOriginalBox = [XDLUI checkboxWithTitle:@"Delete original MP4s" target:nil action:nil];

    NSStackView *optionsRow = [NSStackView stackViewWithViews:@[
        [XDLUI labelWithText:@"Quality" size:12 weight:NSFontWeightRegular color:XDLTheme.secondaryLabel],
        self.qualityPopup,
        [XDLUI labelWithText:@"Resolution" size:12 weight:NSFontWeightRegular color:XDLTheme.secondaryLabel],
        self.resolutionPopup,
        self.deleteOriginalBox,
    ]];
    optionsRow.spacing = 10;
    [stack addArrangedSubview:optionsRow];

    self.convertButton = [XDLPillButton pillWithTitle:@"Convert" onClick:^{
        [weakSelf startConversion];
    }];
    self.spinner = [[NSProgressIndicator alloc] initWithFrame:NSZeroRect];
    self.spinner.style = NSProgressIndicatorStyleSpinning;
    self.spinner.controlSize = NSControlSizeSmall;
    self.spinner.displayedWhenStopped = NO;
    self.spinner.translatesAutoresizingMaskIntoConstraints = NO;
    self.statusLabel = [XDLUI labelWithText:@"" size:11.5 weight:NSFontWeightRegular color:XDLTheme.secondaryLabel];
    [self.statusLabel setContentHuggingPriority:100
                                 forOrientation:NSLayoutConstraintOrientationHorizontal];
    NSStackView *actionRow = [NSStackView stackViewWithViews:@[ self.convertButton, self.spinner, self.statusLabel ]];
    actionRow.spacing = 10;
    actionRow.distribution = NSStackViewDistributionFill;
    [stack addArrangedSubview:actionRow];
    [actionRow.widthAnchor constraintEqualToAnchor:stack.widthAnchor].active = YES;

    [NSNotificationCenter.defaultCenter addObserver:self
                                           selector:@selector(coreEvent:)
                                               name:XDLCoreEventNotification
                                             object:nil];
}

- (void)dealloc {
    [NSNotificationCenter.defaultCenter removeObserver:self];
}

- (void)chooseFolder {
    NSOpenPanel *panel = [NSOpenPanel openPanel];
    panel.canChooseFiles = NO;
    panel.canChooseDirectories = YES;
    __weak typeof(self) weakSelf = self;
    [panel beginSheetModalForWindow:self.view.window completionHandler:^(NSModalResponse result) {
        if (result == NSModalResponseOK && panel.URL) {
            weakSelf.folderField.stringValue = panel.URL.path;
        }
    }];
}

- (void)startConversion {
    if (self.converting || self.folderField.stringValue.length == 0) {
        if (self.folderField.stringValue.length == 0) {
            self.statusLabel.stringValue = @"Choose a folder first.";
        }
        return;
    }
    NSArray *qualities = @[ @"fast", @"better" ];
    NSArray *resolutions = @[ @"original", @"high", @"medium", @"low" ];

    self.converting = YES;
    self.convertButton.enabled = NO;
    [self.spinner startAnimation:nil];
    self.statusLabel.stringValue = @"Converting…";

    __weak typeof(self) weakSelf = self;
    [self.client post:@"/gifs/convert" body:@{
        @"folder_path": self.folderField.stringValue,
        @"quality": qualities[self.qualityPopup.indexOfSelectedItem],
        @"resolution": resolutions[self.resolutionPopup.indexOfSelectedItem],
        @"delete_original": (self.deleteOriginalBox.state == NSControlStateValueOn) ? @YES : @NO,
    } completion:^(id json, NSError *error) {
        if (error) {
            weakSelf.converting = NO;
            weakSelf.convertButton.enabled = YES;
            [weakSelf.spinner stopAnimation:nil];
            weakSelf.statusLabel.stringValue = error.localizedDescription;
        }
    }];
}

- (void)coreEvent:(NSNotification *)notification {
    if (![notification.userInfo[@"type"] isEqualToString:@"gifs-finished"]) {
        return;
    }
    id data = notification.userInfo[@"data"];
    self.converting = NO;
    self.convertButton.enabled = YES;
    [self.spinner stopAnimation:nil];
    if ([data isKindOfClass:NSDictionary.class]) {
        NSString *errorMessage = data[@"error"];
        if ([errorMessage isKindOfClass:NSString.class] && errorMessage.length > 0) {
            self.statusLabel.stringValue = errorMessage;
        } else {
            self.statusLabel.stringValue = [NSString stringWithFormat:@"Converted %@, %@ failed",
                                            data[@"converted"] ?: @0, data[@"failed"] ?: @0];
        }
    }
}

@end
