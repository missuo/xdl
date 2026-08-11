#import "XDLSettingsViewController.h"
#import "XDLCoreClient.h"
#import "XDLComponents.h"
#import "XDLSettings.h"
#import "XDLTheme.h"

@interface XDLSettingsViewController () <NSTextFieldDelegate>
@property (nonatomic) XDLCoreClient *client;
@property (nonatomic) NSSecureTextField *tokenField;
@property (nonatomic) NSTextField *tokenStatusLabel;
@property (nonatomic) BOOL tokenLoaded;
@property (nonatomic) NSTextField *downloadPathField;
@property (nonatomic) NSStepper *concurrencyStepper;
@property (nonatomic) NSTextField *concurrencyValueLabel;
@property (nonatomic) NSStepper *retryStepper;
@property (nonatomic) NSTextField *retryValueLabel;
@property (nonatomic) NSButton *skipExistingBox;
@property (nonatomic) NSButton *deleteIncompleteBox;
@property (nonatomic) NSStepper *timeoutStepper;
@property (nonatomic) NSTextField *timeoutValueLabel;
@property (nonatomic) NSPopUpButton *videoQualityPopup;
@property (nonatomic) NSPopUpButton *imageSizePopup;
@property (nonatomic) NSTextField *filenameTemplateField;
@property (nonatomic) NSTextField *folderTemplateField;
@property (nonatomic) NSButton *autoConvertBox;
@end

@implementation XDLSettingsViewController

- (instancetype)initWithClient:(XDLCoreClient *)client {
    self = [super initWithTitle:@"Settings"];
    if (self) {
        _client = client;
    }
    return self;
}

- (void)viewDidLoad {
    [super viewDidLoad];
    XDLSettings *settings = XDLSettings.shared;
    __weak typeof(self) weakSelf = self;

    // --- Account ---
    NSView *accountCard = nil;
    NSStackView *account = [self addCardWithCaption:@"X Account"
                                        belowAnchor:self.contentTopAnchor
                                                gap:6
                                            outCard:&accountCard];

    NSTextField *tokenBlurb = [XDLUI labelWithText:
        @"The auth_token cookie from x.com. Stored in the macOS Keychain; used by fetches and browser-extension downloads."
                                              size:11.5 weight:NSFontWeightRegular color:XDLTheme.secondaryLabel];
    tokenBlurb.lineBreakMode = NSLineBreakByWordWrapping;
    tokenBlurb.maximumNumberOfLines = 0;
    [account addArrangedSubview:tokenBlurb];
    [tokenBlurb.widthAnchor constraintEqualToAnchor:account.widthAnchor].active = YES;

    self.tokenField = [XDLUI secureFieldWithPlaceholder:@"auth_token"];
    [self.tokenField setContentHuggingPriority:100
                                forOrientation:NSLayoutConstraintOrientationHorizontal];
    XDLPillButton *saveTokenButton = [XDLPillButton pillWithTitle:@"Save" onClick:^{
        [weakSelf saveToken];
    }];
    self.tokenStatusLabel = [XDLUI labelWithText:@"" size:11.5 weight:NSFontWeightRegular color:XDLTheme.secondaryLabel];
    NSStackView *tokenRow = [NSStackView stackViewWithViews:@[
        self.tokenField, saveTokenButton, self.tokenStatusLabel
    ]];
    tokenRow.spacing = 10;
    tokenRow.distribution = NSStackViewDistributionFill;
    [account addArrangedSubview:tokenRow];
    [NSLayoutConstraint activateConstraints:@[
        [tokenRow.widthAnchor constraintEqualToAnchor:account.widthAnchor],
        [self.tokenStatusLabel.widthAnchor constraintGreaterThanOrEqualToConstant:120],
    ]];

    // --- Download ---
    NSView *downloadCard = nil;
    NSStackView *download = [self addCardWithCaption:@"Download"
                                         belowAnchor:accountCard.bottomAnchor
                                                 gap:18
                                             outCard:&downloadCard];

    self.downloadPathField = [XDLUI fieldWithPlaceholder:@"Default: ~/Pictures"];
    self.downloadPathField.stringValue = settings.downloadPath;
    self.downloadPathField.editable = NO;
    XDLPillButton *choosePath = [XDLPillButton pillWithTitle:@"Choose…" onClick:^{
        [weakSelf chooseDownloadPath];
    }];
    XDLPillButton *resetPath = [XDLPillButton pillWithTitle:@"Reset" onClick:^{
        XDLSettings.shared.downloadPath = @"";
        weakSelf.downloadPathField.stringValue = @"";
    }];
    [self.downloadPathField setContentHuggingPriority:100
                                       forOrientation:NSLayoutConstraintOrientationHorizontal];
    NSStackView *pathRow = [NSStackView stackViewWithViews:@[
        [XDLUI labelWithText:@"Folder" size:12 weight:NSFontWeightRegular color:XDLTheme.secondaryLabel],
        self.downloadPathField, choosePath, resetPath
    ]];
    pathRow.spacing = 10;
    pathRow.distribution = NSStackViewDistributionFill;
    [download addArrangedSubview:pathRow];
    [pathRow.widthAnchor constraintEqualToAnchor:download.widthAnchor].active = YES;

    self.concurrencyValueLabel = [XDLUI labelWithText:[NSString stringWithFormat:@"%ld", (long)settings.concurrentDownloads]
                                                 size:12 weight:NSFontWeightMedium color:XDLTheme.label];
    self.concurrencyStepper = [[NSStepper alloc] initWithFrame:NSZeroRect];
    self.concurrencyStepper.minValue = 1;
    self.concurrencyStepper.maxValue = 50;
    self.concurrencyStepper.increment = 1;
    self.concurrencyStepper.integerValue = settings.concurrentDownloads;
    self.concurrencyStepper.target = self;
    self.concurrencyStepper.action = @selector(concurrencyChanged:);
    self.concurrencyStepper.translatesAutoresizingMaskIntoConstraints = NO;

    self.retryValueLabel = [XDLUI labelWithText:[NSString stringWithFormat:@"%ld", (long)settings.retryAttempts]
                                           size:12 weight:NSFontWeightMedium color:XDLTheme.label];
    self.retryStepper = [[NSStepper alloc] initWithFrame:NSZeroRect];
    self.retryStepper.minValue = 1;
    self.retryStepper.maxValue = 5;
    self.retryStepper.increment = 1;
    self.retryStepper.integerValue = settings.retryAttempts;
    self.retryStepper.target = self;
    self.retryStepper.action = @selector(retryChanged:);
    self.retryStepper.translatesAutoresizingMaskIntoConstraints = NO;

    NSStackView *numbersRow = [NSStackView stackViewWithViews:@[
        [XDLUI labelWithText:@"Concurrent downloads" size:12 weight:NSFontWeightRegular color:XDLTheme.secondaryLabel],
        self.concurrencyValueLabel, self.concurrencyStepper,
        [XDLUI labelWithText:@"Retry attempts" size:12 weight:NSFontWeightRegular color:XDLTheme.secondaryLabel],
        self.retryValueLabel, self.retryStepper,
    ]];
    numbersRow.spacing = 10;
    [download addArrangedSubview:numbersRow];

    self.skipExistingBox = [XDLUI checkboxWithTitle:@"Skip existing files"
                                             target:self action:@selector(togglesChanged:)];
    self.skipExistingBox.state = settings.skipExistingFiles ? NSControlStateValueOn : NSControlStateValueOff;
    self.deleteIncompleteBox = [XDLUI checkboxWithTitle:@"Delete incomplete files on failure"
                                                 target:self action:@selector(togglesChanged:)];
    self.deleteIncompleteBox.state = settings.deleteIncompleteFiles ? NSControlStateValueOn : NSControlStateValueOff;
    NSStackView *togglesRow = [NSStackView stackViewWithViews:@[ self.skipExistingBox, self.deleteIncompleteBox ]];
    togglesRow.spacing = 14;
    [download addArrangedSubview:togglesRow];

    // --- Quality ---
    NSView *qualityCard = nil;
    NSStackView *quality = [self addCardWithCaption:@"Media Quality"
                                        belowAnchor:downloadCard.bottomAnchor
                                                gap:18
                                            outCard:&qualityCard];

    self.videoQualityPopup = [XDLUI popupWithItems:@[ @"Highest", @"720p", @"480p", @"360p", @"Lowest" ]];
    NSArray *videoQualities = @[ @"highest", @"720", @"480", @"360", @"lowest" ];
    NSUInteger videoIndex = [videoQualities indexOfObject:settings.videoQuality];
    [self.videoQualityPopup selectItemAtIndex:videoIndex == NSNotFound ? 0 : videoIndex];
    self.videoQualityPopup.target = self;
    self.videoQualityPopup.action = @selector(qualityChanged:);

    self.imageSizePopup = [XDLUI popupWithItems:@[ @"Original", @"Large", @"Medium", @"Small" ]];
    NSArray *imageSizes = @[ @"orig", @"large", @"medium", @"small" ];
    NSUInteger imageIndex = [imageSizes indexOfObject:settings.imageSize];
    [self.imageSizePopup selectItemAtIndex:imageIndex == NSNotFound ? 0 : imageIndex];
    self.imageSizePopup.target = self;
    self.imageSizePopup.action = @selector(qualityChanged:);

    NSStackView *qualityRow = [NSStackView stackViewWithViews:@[
        [XDLUI labelWithText:@"Video quality" size:12 weight:NSFontWeightRegular color:XDLTheme.secondaryLabel],
        self.videoQualityPopup,
        [XDLUI labelWithText:@"Image size" size:12 weight:NSFontWeightRegular color:XDLTheme.secondaryLabel],
        self.imageSizePopup,
    ]];
    qualityRow.spacing = 10;
    [quality addArrangedSubview:qualityRow];

    // --- Naming ---
    NSView *namingCard = nil;
    NSStackView *naming = [self addCardWithCaption:@"File Naming"
                                       belowAnchor:qualityCard.bottomAnchor
                                               gap:18
                                           outCard:&namingCard];

    NSTextField *tokens = [XDLUI labelWithText:@"Tokens: {handle} {name} {date} {tweet_id} {index} {media_id} {type}"
                                          size:11 weight:NSFontWeightRegular color:XDLTheme.secondaryLabel];
    [naming addArrangedSubview:tokens];

    self.filenameTemplateField = [XDLUI fieldWithPlaceholder:@"{handle}_{date}_{tweet_id}"];
    self.filenameTemplateField.stringValue = settings.filenameTemplate;
    self.filenameTemplateField.delegate = self;
    self.folderTemplateField = [XDLUI fieldWithPlaceholder:@"{handle}"];
    self.folderTemplateField.stringValue = settings.folderTemplate;
    self.folderTemplateField.delegate = self;
    NSStackView *templatesRow = [NSStackView stackViewWithViews:@[
        [XDLUI labelWithText:@"Filename" size:12 weight:NSFontWeightRegular color:XDLTheme.secondaryLabel],
        self.filenameTemplateField,
        [XDLUI labelWithText:@"Folder" size:12 weight:NSFontWeightRegular color:XDLTheme.secondaryLabel],
        self.folderTemplateField,
    ]];
    templatesRow.spacing = 10;
    templatesRow.distribution = NSStackViewDistributionFill;
    // Filename field flexes; folder field is fixed so the row is determinate.
    [self.filenameTemplateField setContentHuggingPriority:100
                                          forOrientation:NSLayoutConstraintOrientationHorizontal];
    [naming addArrangedSubview:templatesRow];
    [NSLayoutConstraint activateConstraints:@[
        [templatesRow.widthAnchor constraintEqualToAnchor:naming.widthAnchor],
        [self.filenameTemplateField.widthAnchor constraintGreaterThanOrEqualToConstant:240],
        [self.folderTemplateField.widthAnchor constraintEqualToConstant:160],
    ]];

    // --- Fetching & GIF ---
    NSView *networkCard = nil;
    NSStackView *network = [self addCardWithCaption:@"Fetching & GIF"
                                        belowAnchor:namingCard.bottomAnchor
                                                gap:18
                                            outCard:&networkCard];

    self.timeoutValueLabel = [XDLUI labelWithText:[NSString stringWithFormat:@"%lds", (long)settings.fetchTimeout]
                                             size:12 weight:NSFontWeightMedium color:XDLTheme.label];
    self.timeoutStepper = [[NSStepper alloc] initWithFrame:NSZeroRect];
    self.timeoutStepper.minValue = 10;
    self.timeoutStepper.maxValue = 600;
    self.timeoutStepper.increment = 10;
    self.timeoutStepper.integerValue = settings.fetchTimeout;
    self.timeoutStepper.target = self;
    self.timeoutStepper.action = @selector(timeoutChanged:);
    self.timeoutStepper.translatesAutoresizingMaskIntoConstraints = NO;
    NSStackView *timeoutRow = [NSStackView stackViewWithViews:@[
        [XDLUI labelWithText:@"Fetch timeout (per page)" size:12 weight:NSFontWeightRegular color:XDLTheme.secondaryLabel],
        self.timeoutValueLabel, self.timeoutStepper,
    ]];
    timeoutRow.spacing = 10;
    [network addArrangedSubview:timeoutRow];

    self.autoConvertBox = [XDLUI checkboxWithTitle:@"Auto-convert GIF MP4s after download"
                                            target:self action:@selector(togglesChanged:)];
    self.autoConvertBox.state = settings.autoConvertGifs ? NSControlStateValueOn : NSControlStateValueOff;
    [network addArrangedSubview:self.autoConvertBox];

    [networkCard.bottomAnchor constraintLessThanOrEqualToAnchor:self.contentView.bottomAnchor
                                                       constant:-XDLPageContentMargin].active = YES;
}

- (void)viewDidAppear {
    [super viewDidAppear];
    [self loadTokenIfNeeded];
}

- (void)loadTokenIfNeeded {
    if (self.tokenLoaded || self.client.state != XDLCoreStateReady) {
        return;
    }
    __weak typeof(self) weakSelf = self;
    [self.client get:@"/authtoken?slot=public" completion:^(id json, NSError *error) {
        __strong typeof(weakSelf) self = weakSelf;
        if (!self) return;
        NSString *token = [json isKindOfClass:NSDictionary.class] ? json[@"token"] : nil;
        if ([token isKindOfClass:NSString.class] && token.length > 0) {
            self.tokenLoaded = YES;
            if (self.tokenField.stringValue.length == 0) {
                self.tokenField.stringValue = token;
            }
            self.tokenStatusLabel.stringValue = @"Loaded from Keychain";
        }
    }];
}

- (void)saveToken {
    __weak typeof(self) weakSelf = self;
    [self.client post:@"/authtoken"
                 body:@{ @"slot": @"public", @"token": self.tokenField.stringValue }
           completion:^(id json, NSError *error) {
        __strong typeof(weakSelf) self = weakSelf;
        if (!self) return;
        if (error) {
            self.tokenStatusLabel.stringValue = error.localizedDescription;
            self.tokenStatusLabel.textColor = XDLTheme.statusRed;
        } else {
            self.tokenStatusLabel.stringValue = self.tokenField.stringValue.length ? @"Saved to Keychain" : @"Cleared";
            self.tokenStatusLabel.textColor = XDLTheme.statusGreen;
        }
    }];
}

- (void)chooseDownloadPath {
    NSOpenPanel *panel = [NSOpenPanel openPanel];
    panel.canChooseFiles = NO;
    panel.canChooseDirectories = YES;
    panel.canCreateDirectories = YES;
    __weak typeof(self) weakSelf = self;
    [panel beginSheetModalForWindow:self.view.window completionHandler:^(NSModalResponse result) {
        if (result == NSModalResponseOK && panel.URL) {
            XDLSettings.shared.downloadPath = panel.URL.path;
            weakSelf.downloadPathField.stringValue = panel.URL.path;
        }
    }];
}

- (void)concurrencyChanged:(NSStepper *)sender {
    XDLSettings.shared.concurrentDownloads = sender.integerValue;
    self.concurrencyValueLabel.stringValue = [NSString stringWithFormat:@"%ld", (long)sender.integerValue];
}

- (void)retryChanged:(NSStepper *)sender {
    XDLSettings.shared.retryAttempts = sender.integerValue;
    self.retryValueLabel.stringValue = [NSString stringWithFormat:@"%ld", (long)sender.integerValue];
}

- (void)timeoutChanged:(NSStepper *)sender {
    XDLSettings.shared.fetchTimeout = sender.integerValue;
    self.timeoutValueLabel.stringValue = [NSString stringWithFormat:@"%lds", (long)sender.integerValue];
}

- (void)togglesChanged:(id)sender {
    XDLSettings *settings = XDLSettings.shared;
    settings.skipExistingFiles = self.skipExistingBox.state == NSControlStateValueOn;
    settings.deleteIncompleteFiles = self.deleteIncompleteBox.state == NSControlStateValueOn;
    settings.autoConvertGifs = self.autoConvertBox.state == NSControlStateValueOn;
}

- (void)qualityChanged:(id)sender {
    XDLSettings *settings = XDLSettings.shared;
    NSArray *videoQualities = @[ @"highest", @"720", @"480", @"360", @"lowest" ];
    NSArray *imageSizes = @[ @"orig", @"large", @"medium", @"small" ];
    settings.videoQuality = videoQualities[self.videoQualityPopup.indexOfSelectedItem];
    settings.imageSize = imageSizes[self.imageSizePopup.indexOfSelectedItem];
}

- (void)controlTextDidChange:(NSNotification *)notification {
    XDLSettings *settings = XDLSettings.shared;
    if (notification.object == self.filenameTemplateField) {
        settings.filenameTemplate = self.filenameTemplateField.stringValue;
    } else if (notification.object == self.folderTemplateField) {
        settings.folderTemplate = self.folderTemplateField.stringValue;
    }
}

@end
