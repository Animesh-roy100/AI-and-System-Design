from typing import TypedDict, Annotated, List, Optional
from langchain_core.messages import BaseMessage
import operator

class GraphState(TypedDict):
    """
    Represents the state of our graph.
    
    Attributes:
        messages: List of messages (Human, AI, Tool, etc.). Reduced using operator.add.
        category: The resolved intent categorization of the user's issue.
        requires_human_approval: Flag indicating if a manual check is needed.
        final_response: The assembled answer after execution.
    """
    messages: Annotated[List[BaseMessage], operator.add]
    category: Optional[str]
    requires_human_approval: Optional[bool]
    final_response: Optional[str]
