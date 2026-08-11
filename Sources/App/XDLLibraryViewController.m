#import "XDLLibraryViewController.h"
#import <UniformTypeIdentifiers/UniformTypeIdentifiers.h>
#import "XDLCoreClient.h"
#import "XDLComponents.h"
#import "XDLTheme.h"

@interface XDLLibraryViewController () <NSTableViewDataSource, NSTableViewDelegate>
@property (nonatomic) XDLCoreClient *client;
@property (nonatomic) NSTableView *tableView;
@property (nonatomic) NSArray<NSDictionary *> *accounts;
@property (nonatomic) NSTextField *statusLabel;
@property (nonatomic) XDLPillButton *openButton;
@property (nonatomic) XDLPillButton *deleteButton;
@property (nonatomic) XDLPillButton *exportButton;
@end

@implementation XDLLibraryViewController

- (instancetype)initWithClient:(XDLCoreClient *)client {
    self = [super initWithTitle:@"Library"];
    if (self) {
        _client = client;
        _accounts = @[];
    }
    return self;
}

- (void)viewDidLoad {
    [super viewDidLoad];

    NSView *card = nil;
    NSStackView *stack = [self addCardWithCaption:@"Saved Accounts"
                                      belowAnchor:self.contentTopAnchor
                                              gap:6
                                          outCard:&card];

    __weak typeof(self) weakSelf = self;
    XDLPillButton *refresh = [XDLPillButton pillWithTitle:@"Refresh" onClick:^{
        [weakSelf reload];
    }];
    self.openButton = [XDLPillButton pillWithTitle:@"Open in Download" onClick:^{
        [weakSelf openSelected];
    }];
    XDLPillButton *refetchButton = [XDLPillButton pillWithTitle:@"Refetch" onClick:^{
        [weakSelf refetchSelected];
    }];
    self.deleteButton = [XDLPillButton pillWithTitle:@"Delete" onClick:^{
        [weakSelf deleteSelected];
    }];
    self.exportButton = [XDLPillButton pillWithTitle:@"Export JSON" onClick:^{
        [weakSelf exportSelected];
    }];
    XDLPillButton *importButton = [XDLPillButton pillWithTitle:@"Import JSON" onClick:^{
        [weakSelf importAccount];
    }];
    self.statusLabel = [XDLUI labelWithText:@"" size:11.5 weight:NSFontWeightRegular color:XDLTheme.secondaryLabel];

    NSStackView *toolbar = [NSStackView stackViewWithViews:@[
        refresh, self.openButton, refetchButton, self.deleteButton, self.exportButton, importButton, self.statusLabel
    ]];
    toolbar.spacing = 10;
    toolbar.distribution = NSStackViewDistributionFill;
    // The status label is the designated flex view; every pill hugs its content.
    [self.statusLabel setContentHuggingPriority:100
                                forOrientation:NSLayoutConstraintOrientationHorizontal];
    [stack addArrangedSubview:toolbar];
    [toolbar.widthAnchor constraintEqualToAnchor:stack.widthAnchor].active = YES;

    self.tableView = [[NSTableView alloc] initWithFrame:NSZeroRect];
    self.tableView.dataSource = self;
    self.tableView.delegate = self;
    self.tableView.rowHeight = 26;
    self.tableView.backgroundColor = NSColor.clearColor;
    self.tableView.doubleAction = @selector(tableDoubleClicked:);
    self.tableView.target = self;

    NSMenu *menu = [[NSMenu alloc] init];
    NSMenuItem *copyItem = [menu addItemWithTitle:@"Copy Username" action:@selector(copyUsernameFromMenu:) keyEquivalent:@""];
    copyItem.target = self;
    NSMenuItem *copyURLItem = [menu addItemWithTitle:@"Copy Profile URL" action:@selector(copyProfileURLFromMenu:) keyEquivalent:@""];
    copyURLItem.target = self;
    [menu addItem:[NSMenuItem separatorItem]];
    NSMenuItem *openItem = [menu addItemWithTitle:@"Open in Download" action:@selector(openFromMenu:) keyEquivalent:@""];
    openItem.target = self;
    NSMenuItem *refetchItem = [menu addItemWithTitle:@"Refetch" action:@selector(refetchFromMenu:) keyEquivalent:@""];
    refetchItem.target = self;
    [menu addItem:[NSMenuItem separatorItem]];
    NSMenuItem *deleteItem = [menu addItemWithTitle:@"Delete…" action:@selector(deleteFromMenu:) keyEquivalent:@""];
    deleteItem.target = self;
    self.tableView.menu = menu;

    struct {
        const char *identifier;
        const char *title;
        CGFloat width;
    } columns[] = {
        { "username", "Username", 150 },
        { "name", "Name", 160 },
        { "media_type", "Type", 90 },
        { "total", "Media", 70 },
        { "fetched", "Last Fetched", 150 },
        { "group", "Group", 100 },
    };
    for (size_t i = 0; i < sizeof(columns) / sizeof(columns[0]); i++) {
        NSTableColumn *column = [[NSTableColumn alloc] initWithIdentifier:@(columns[i].identifier)];
        column.title = @(columns[i].title);
        column.width = columns[i].width;
        [self.tableView addTableColumn:column];
    }

    NSScrollView *scroll = [[NSScrollView alloc] initWithFrame:NSZeroRect];
    scroll.documentView = self.tableView;
    scroll.hasVerticalScroller = YES;
    scroll.drawsBackground = NO;
    scroll.borderType = NSNoBorder;
    scroll.translatesAutoresizingMaskIntoConstraints = NO;
    [scroll setContentHuggingPriority:100
                       forOrientation:NSLayoutConstraintOrientationVertical];
    [stack addArrangedSubview:scroll];
    [NSLayoutConstraint activateConstraints:@[
        [scroll.heightAnchor constraintGreaterThanOrEqualToConstant:320],
        [scroll.widthAnchor constraintEqualToAnchor:stack.widthAnchor],

        [self.contentView.heightAnchor constraintGreaterThanOrEqualToAnchor:self.viewportHeightAnchor],
        [card.bottomAnchor constraintEqualToAnchor:self.contentView.bottomAnchor
                                         constant:-XDLPageContentMargin],
    ]];

    [NSNotificationCenter.defaultCenter addObserver:self
                                           selector:@selector(reload)
                                               name:XDLCoreStateDidChangeNotification
                                             object:nil];
    [NSNotificationCenter.defaultCenter addObserver:self
                                           selector:@selector(coreEvent:)
                                               name:XDLCoreEventNotification
                                             object:nil];
    [self reload];
}

// Browser-initiated jobs update the library from outside this page.
- (void)coreEvent:(NSNotification *)notification {
    NSString *type = notification.userInfo[@"type"];
    if ([type isEqualToString:@"accounts-changed"]) {
        [self reload];
        return;
    }
    if ([type isEqualToString:@"browser-job"]) {
        id data = notification.userInfo[@"data"];
        NSString *stage = [data isKindOfClass:NSDictionary.class] ? data[@"stage"] : nil;
        NSString *username = [data isKindOfClass:NSDictionary.class] ? data[@"username"] : nil;
        if ([stage isEqualToString:@"fetching"]) {
            self.statusLabel.stringValue = [NSString stringWithFormat:@"Browser job: fetching @%@…", username];
        } else if ([stage isEqualToString:@"downloading"]) {
            self.statusLabel.stringValue = [NSString stringWithFormat:@"Browser job: downloading @%@…", username];
        } else if ([stage isEqualToString:@"done"] || [stage isEqualToString:@"error"]) {
            self.statusLabel.stringValue = @"";
            [self reload];
        }
    }
}

- (void)dealloc {
    [NSNotificationCenter.defaultCenter removeObserver:self];
}

- (void)viewDidAppear {
    [super viewDidAppear];
    [self reload];
}

- (void)reload {
    if (self.client.state != XDLCoreStateReady) {
        return;
    }
    __weak typeof(self) weakSelf = self;
    [self.client get:@"/accounts" completion:^(id json, NSError *error) {
        __strong typeof(weakSelf) self = weakSelf;
        if (!self) return;
        if (error) {
            self.statusLabel.stringValue = error.localizedDescription;
            return;
        }
        self.accounts = [json isKindOfClass:NSArray.class] ? json : @[];
        self.statusLabel.stringValue = [NSString stringWithFormat:@"%lu accounts", (unsigned long)self.accounts.count];
        [self.tableView reloadData];
    }];
}

- (nullable NSDictionary *)selectedAccount {
    NSInteger row = self.tableView.selectedRow;
    if (row < 0 || row >= (NSInteger)self.accounts.count) {
        return nil;
    }
    return self.accounts[row];
}

- (void)openSelected {
    NSDictionary *account = [self selectedAccount];
    if (!account || !self.onOpenAccount) {
        return;
    }
    NSString *username = account[@"username"] ?: @"";
    NSString *mediaType = account[@"media_type"] ?: @"media";
    self.onOpenAccount(username, mediaType);
}

- (void)tableDoubleClicked:(id)sender {
    [self openSelected];
}

- (void)refetchSelected {
    NSDictionary *account = [self selectedAccount];
    if (account && self.onRefetchAccount) {
        self.onRefetchAccount(account[@"username"] ?: @"", account[@"media_type"] ?: @"media");
    }
}

// Context-menu actions operate on the clicked row (falling back to the selection).
- (nullable NSDictionary *)menuAccount {
    NSInteger row = self.tableView.clickedRow;
    if (row < 0) {
        row = self.tableView.selectedRow;
    }
    if (row < 0 || row >= (NSInteger)self.accounts.count) {
        return nil;
    }
    return self.accounts[row];
}

- (void)copyUsernameFromMenu:(id)sender {
    NSDictionary *account = [self menuAccount];
    NSString *username = account[@"username"];
    if (![username isKindOfClass:NSString.class]) {
        return;
    }
    [NSPasteboard.generalPasteboard clearContents];
    [NSPasteboard.generalPasteboard setString:username forType:NSPasteboardTypeString];
    self.statusLabel.stringValue = [NSString stringWithFormat:@"Copied @%@", username];
}

- (void)copyProfileURLFromMenu:(id)sender {
    NSDictionary *account = [self menuAccount];
    NSString *username = account[@"username"];
    if (![username isKindOfClass:NSString.class]) {
        return;
    }
    [NSPasteboard.generalPasteboard clearContents];
    [NSPasteboard.generalPasteboard setString:[@"https://x.com/" stringByAppendingString:username]
                                      forType:NSPasteboardTypeString];
    self.statusLabel.stringValue = @"Copied profile URL";
}

- (void)openFromMenu:(id)sender {
    NSDictionary *account = [self menuAccount];
    if (account && self.onOpenAccount) {
        self.onOpenAccount(account[@"username"] ?: @"", account[@"media_type"] ?: @"media");
    }
}

- (void)refetchFromMenu:(id)sender {
    NSDictionary *account = [self menuAccount];
    if (account && self.onRefetchAccount) {
        self.onRefetchAccount(account[@"username"] ?: @"", account[@"media_type"] ?: @"media");
    }
}

- (void)deleteFromMenu:(id)sender {
    NSDictionary *account = [self menuAccount];
    if (!account) {
        return;
    }
    NSInteger row = [self.accounts indexOfObject:account];
    if (row != NSNotFound) {
        [self.tableView selectRowIndexes:[NSIndexSet indexSetWithIndex:row] byExtendingSelection:NO];
    }
    [self deleteSelected];
}

- (void)deleteSelected {
    NSDictionary *account = [self selectedAccount];
    if (!account) {
        return;
    }
    NSAlert *alert = [[NSAlert alloc] init];
    alert.messageText = [NSString stringWithFormat:@"Delete @%@?", account[@"username"]];
    alert.informativeText = @"Removes the saved fetch data from the library. Downloaded files are not touched.";
    [alert addButtonWithTitle:@"Delete"];
    [alert addButtonWithTitle:@"Cancel"];
    if ([alert runModal] != NSAlertFirstButtonReturn) {
        return;
    }
    __weak typeof(self) weakSelf = self;
    [self.client post:@"/accounts/delete"
                 body:@{ @"id": account[@"id"] ?: @0 }
           completion:^(id json, NSError *error) {
        [weakSelf reload];
    }];
}

- (void)exportSelected {
    NSDictionary *account = [self selectedAccount];
    if (!account) {
        return;
    }
    NSOpenPanel *panel = [NSOpenPanel openPanel];
    panel.canChooseFiles = NO;
    panel.canChooseDirectories = YES;
    panel.canCreateDirectories = YES;
    panel.prompt = @"Export Here";
    __weak typeof(self) weakSelf = self;
    [panel beginSheetModalForWindow:self.view.window completionHandler:^(NSModalResponse result) {
        if (result != NSModalResponseOK || !panel.URL) {
            return;
        }
        [weakSelf.client post:@"/accounts/export-json"
                         body:@{ @"id": account[@"id"] ?: @0, @"output_dir": panel.URL.path }
                   completion:^(id json, NSError *error) {
            if (error) {
                weakSelf.statusLabel.stringValue = error.localizedDescription;
                return;
            }
            NSString *path = [json isKindOfClass:NSDictionary.class] ? json[@"path"] : nil;
            weakSelf.statusLabel.stringValue = @"Exported";
            if ([path isKindOfClass:NSString.class]) {
                [NSWorkspace.sharedWorkspace activateFileViewerSelectingURLs:@[ [NSURL fileURLWithPath:path] ]];
            }
        }];
    }];
}

- (void)importAccount {
    NSOpenPanel *panel = [NSOpenPanel openPanel];
    panel.canChooseFiles = YES;
    panel.canChooseDirectories = NO;
    panel.allowedContentTypes = @[ [UTType typeWithFilenameExtension:@"json"] ];
    __weak typeof(self) weakSelf = self;
    [panel beginSheetModalForWindow:self.view.window completionHandler:^(NSModalResponse result) {
        if (result != NSModalResponseOK || !panel.URL) {
            return;
        }
        [weakSelf.client post:@"/accounts/import"
                         body:@{ @"path": panel.URL.path }
                   completion:^(id json, NSError *error) {
            if (error) {
                weakSelf.statusLabel.stringValue = error.localizedDescription;
                return;
            }
            [weakSelf reload];
        }];
    }];
}

#pragma mark - Table

- (NSInteger)numberOfRowsInTableView:(NSTableView *)tableView {
    return self.accounts.count;
}

- (NSTableRowView *)tableView:(NSTableView *)tableView rowViewForRow:(NSInteger)row {
    return [[XDLTableRowView alloc] initWithFrame:NSZeroRect];
}

- (NSView *)tableView:(NSTableView *)tableView viewForTableColumn:(NSTableColumn *)tableColumn row:(NSInteger)row {
    NSString *identifier = tableColumn.identifier;
    NSDictionary *account = self.accounts[row];

    XDLTableCell *cell = [tableView makeViewWithIdentifier:identifier owner:self];
    if (!cell) {
        cell = [[XDLTableCell alloc] initWithFrame:NSZeroRect];
        cell.identifier = identifier;
    }
    NSTextField *label = cell.label;
    label.font = [NSFont systemFontOfSize:12 weight:NSFontWeightRegular];
    label.textColor = XDLTheme.label;

    if ([identifier isEqualToString:@"username"]) {
        label.stringValue = [NSString stringWithFormat:@"@%@", account[@"username"] ?: @""];
    } else if ([identifier isEqualToString:@"name"]) {
        label.stringValue = [account[@"name"] isKindOfClass:NSString.class] ? account[@"name"] : @"";
    } else if ([identifier isEqualToString:@"media_type"]) {
        label.stringValue = [account[@"media_type"] isKindOfClass:NSString.class] ? account[@"media_type"] : @"";
        label.textColor = XDLTheme.secondaryLabel;
    } else if ([identifier isEqualToString:@"total"]) {
        label.stringValue = [NSString stringWithFormat:@"%@", account[@"total_media"] ?: @"0"];
        label.font = [NSFont monospacedDigitSystemFontOfSize:12 weight:NSFontWeightRegular];
    } else if ([identifier isEqualToString:@"fetched"]) {
        label.stringValue = [account[@"last_fetched"] isKindOfClass:NSString.class] ? account[@"last_fetched"] : @"";
        label.textColor = XDLTheme.secondaryLabel;
    } else if ([identifier isEqualToString:@"group"]) {
        label.stringValue = [account[@"group_name"] isKindOfClass:NSString.class] ? account[@"group_name"] : @"";
        label.textColor = XDLTheme.secondaryLabel;
    }
    return cell;
}

@end
