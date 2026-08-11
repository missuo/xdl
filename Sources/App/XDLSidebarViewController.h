#import <Cocoa/Cocoa.h>

NS_ASSUME_NONNULL_BEGIN

/// Hand-rolled Surge-style sidebar: SF Symbol rows with hover + selection.
@interface XDLSidebarViewController : NSViewController

@property (nonatomic, copy, nullable) void (^onSelect)(NSString *itemID);

- (void)selectItemID:(NSString *)itemID;

@end

NS_ASSUME_NONNULL_END
